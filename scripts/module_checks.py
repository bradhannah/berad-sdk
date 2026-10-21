"""Offline, isolated Go module lanes with source-bound selective result reuse."""

from __future__ import annotations

from collections.abc import Callable, Iterator, Sequence
import hashlib
import importlib.util
import json
import os
import platform
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tempfile
from typing import TYPE_CHECKING, NotRequired, TypedDict, cast

if TYPE_CHECKING:
    from scripts.module_inventory import ModuleSpec


class GoModule(TypedDict):
    """Relevant fields of Go's module metadata and replacement records."""

    Path: str
    Version: NotRequired[str]
    Replace: NotRequired[GoModule]


class GoPackage(TypedDict):
    """Fields consumed from streamed go list JSON package records."""

    ImportPath: str
    Dir: NotRequired[str]
    GoFiles: NotRequired[list[str]]
    CgoFiles: NotRequired[list[str]]
    TestGoFiles: NotRequired[list[str]]
    XTestGoFiles: NotRequired[list[str]]
    Module: NotRequired[GoModule]


class TestEvent(TypedDict):
    """Terminal Go test events used to reject incomplete or skipped coverage."""

    Package: str
    Action: str
    Test: NotRequired[str]


class Replacement(TypedDict):
    """A go mod edit replacement pair."""

    Old: GoModule
    New: GoModule


class GoMod(TypedDict):
    """Subset of go mod edit JSON needed for replacement closure validation."""

    Replace: NotRequired[list[Replacement]]


SPEC = importlib.util.spec_from_file_location(
    "module_inventory", Path(__file__).with_name("module_inventory.py")
)
if SPEC is None or SPEC.loader is None:
    raise ImportError("cannot load SDK module inventory")
INVENTORY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(INVENTORY)


def inventory(root: Path, manifest: object, paths: Sequence[Path]) -> list[ModuleSpec]:
    """Check source registration and required module contracts."""
    modules = INVENTORY.normalize(manifest)
    allowed_mods: set[Path] = set()
    for module in modules:
        name = module["id"]
        directory = root / name
        for required in (
            "go.mod",
            "go.sum",
            "LICENSE",
            "README.md",
            "testdata/consumer/go.mod",
        ):
            if not (directory / required).is_file():
                raise ValueError(f"missing module contract/input: {name}/{required}")
        metadata = (directory / "go.mod").read_text()
        if not re.search(
            r"^module " + re.escape(INVENTORY.import_path(module)) + r"\s*$",
            metadata,
            re.M,
        ):
            raise ValueError("module must use its canonical SDK identity")
        if re.search(r"^\s*replace\b", metadata, re.M):
            raise ValueError("published SDK module must not use replacements")
        allowed_mods.update(
            (directory / "go.mod", directory / "testdata/consumer/go.mod")
        )
    for path in paths:
        if INVENTORY.tool_output(path.relative_to(root)):
            continue
        if path.name == "go.work" or path.name == "go.mod" and path not in allowed_mods:
            raise ValueError(
                f"unregistered runtime code/module: {path.relative_to(root)}"
            )
        if path.suffix == ".go":
            relative = path.relative_to(root).parts
            if len(relative) < 3 or "/".join(relative[:2]) not in {
                m["id"] for m in modules
            }:
                raise ValueError(
                    f"unregistered runtime code/module: {path.relative_to(root)}"
                )
            if "//go:embed" in path.read_text():
                raise ValueError(
                    "register embedded asset dependency selection "
                    "before adding go:embed"
                )
    return modules


def run(command: Sequence[str], cwd: Path, env: dict[str, str]) -> str:
    """Run an offline command, preserving diagnostics on failure."""
    result = subprocess.run(
        command,
        cwd=cwd,
        env=env,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=600,
    )
    if result.returncode:
        print(result.stdout, end="")
        print(result.stderr, end="")
        raise subprocess.CalledProcessError(result.returncode, command)
    return result.stdout


def objects(text: str) -> Iterator[GoPackage]:
    """Decode the object stream emitted by go list."""
    decoder = json.JSONDecoder()
    while text.strip():
        item, end = decoder.raw_decode(text.lstrip())
        if not isinstance(item, dict):
            raise ValueError("go list must emit JSON package objects")
        yield cast(GoPackage, item)
        text = text.lstrip()[end:]


def environment() -> dict[str, str]:
    """Select offline execution and reject inherited lane overrides."""
    env = dict(
        os.environ, GOWORK="off", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local"
    )
    if env.get("GOFLAGS"):
        raise ValueError(
            "module checks require empty GOFLAGS; fixed lane flags own test selection"
        )
    return env


def tools(root: Path, env: dict[str, str]) -> str:
    """Bind the selected Go toolchain and compiler implementations."""
    values: dict[str, str] = json.loads(run(["go", "env", "-json"], root, env))
    if values.get("GOFLAGS"):
        raise ValueError("persisted GOFLAGS override registered module checks")
    values["GOGCCFLAGS"] = re.sub(
        r"go-build[0-9]+", "go-build<TEMP>", values.get("GOGCCFLAGS", "")
    )
    digest = hashlib.sha256(json.dumps(values, sort_keys=True).encode())
    digest.update(run(["go", "version"], root, env).encode())
    go_path = shutil.which("go", path=env.get("PATH"))
    if go_path is None:
        raise ValueError("registered checks require a provisioned Go toolchain")
    selected_go = Path(go_path).resolve(strict=True)
    digest.update(str(selected_go).encode())
    digest.update(selected_go.read_bytes())
    # Bind toolchain implementation, not just a version label. Dependency caches
    # remain provisioned inputs checked against each module's go.mod/go.sum.
    goroot = Path(values["GOROOT"])
    for directory in (goroot / "bin", goroot / "pkg/tool", goroot / "src"):
        for path in sorted(directory.rglob("*")):
            if path.is_file():
                digest.update(str(path).encode())
                digest.update(path.read_bytes())
    for name in (values.get("CC", ""), values.get("CXX", "")):
        command = shlex.split(name)
        path = shutil.which(command[0], path=env.get("PATH")) if command else None
        if not path:
            raise ValueError(
                "registered race checks require provisioned C/C++ compilers"
            )
        digest.update(str(Path(path).resolve()).encode())
        digest.update(Path(path).read_bytes())
    return digest.hexdigest()


# Separate dimensions preserve existing callers and make cache inputs explicit.
# pylint: disable-next=too-many-arguments,too-many-positional-arguments
def fingerprint(
    root: Path,
    module: ModuleSpec,
    lane: str,
    policy: str,
    tool_identity: str,
    dependencies: Sequence[ModuleSpec] = (),
) -> str:
    """Identify a lane's source and dependency closure, including native selection."""
    digest = hashlib.sha256(
        json.dumps([module, lane, policy, tool_identity], sort_keys=True).encode()
    )
    if any(
        item.get("execution") == "native-render" for item in (module, *dependencies)
    ):
        # Render lanes need an actual desktop backend. Bind its selection separately
        # from Go tool identity; test success is not physical display acceptance.
        native = {
            name: os.environ.get(name)
            for name in (
                "DISPLAY",
                "WAYLAND_DISPLAY",
                "XAUTHORITY",
                "XDG_SESSION_TYPE",
                "EBITENGINE_GRAPHICS_LIBRARY",
                "EBITENGINE_OPENGL",
                "CGO_ENABLED",
            )
        }
        digest.update(
            json.dumps([platform.platform(), native], sort_keys=True).encode()
        )
    for selected in (module, *dependencies):
        digest.update(selected["id"].encode())
        directory = root / selected["id"]
        for path in sorted(directory.rglob("*")):
            if INVENTORY.tool_output(path.relative_to(root)):
                continue
            if not path.is_file():
                continue
            relative = path.relative_to(directory)
            if relative.parts[:2] == ("testdata", "consumer") and lane not in (
                "consumer",
                "candidate",
            ):
                continue
            if str(relative) in ("README.md", "LICENSE"):
                # Cheap contracts run on every invocation; no embeds allowed.
                continue
            digest.update(str(relative).encode())
            digest.update(str(path.stat().st_mode).encode())
            digest.update(path.read_bytes())
    return digest.hexdigest()


def reusable(record: Path, identity: str) -> bool:
    """Only a complete matching success record is reusable."""
    try:
        return json.loads(record.read_text()) == {
            "schema": 1,
            "state": "passed",
            "identity": identity,
        }
    except (ValueError, OSError):
        return False


def cached_lane(
    cache: Path,
    name: str,
    identity: Callable[[], str],
    execute: Callable[[], None],
    plan: bool = False,
) -> None:
    """Serialize a lane, invalidate failed attempts and bind before/after inputs."""
    record = cache / (name + ".json")
    if plan:
        print(
            f"{name}: {'REUSE' if reusable(record, identity()) else 'RUN'} "
            "(source/dependencies/tools/lane inputs)"
        )
        return
    cache.mkdir(parents=True, exist_ok=True)
    lock = cache / (name + ".running")
    try:
        lock.mkdir()
    except FileExistsError as error:
        raise ValueError(
            f"{name}: validation already running; "
            f"inspect interrupted jobs before removing {lock}"
        ) from error
    try:
        try:
            before = identity()
        except BaseException:
            record.unlink(missing_ok=True)
            raise
        if reusable(record, before):
            print(f"{name}: REUSED (material inputs unchanged)", flush=True)
            return
        record.unlink(missing_ok=True)
        print(f"{name}: RUN (no matching successful result)", flush=True)
        execute()
        if before != identity():
            raise ValueError(f"{name}: inputs changed during execution")
        write_success(record, before)
    finally:
        lock.rmdir()


def write_success(record: Path, identity: str) -> None:
    """Atomically publish the shared complete-success record format."""
    with tempfile.NamedTemporaryFile(
        mode="w", dir=record.parent, delete=False
    ) as output:
        json.dump({"schema": 1, "state": "passed", "identity": identity}, output)
        temporary = Path(output.name)
    os.replace(temporary, record)


def test_packages(directory: Path, env: dict[str, str], race: bool = False) -> None:
    """Run every package and require complete passing test-event coverage."""
    packages = list(
        objects(run(["go", "list", "-mod=readonly", "-json", "./..."], directory, env))
    )
    expected = {item["ImportPath"] for item in packages}
    tested = {
        item["ImportPath"]
        for item in packages
        if item.get("TestGoFiles") or item.get("XTestGoFiles")
    }
    raw = run(
        [
            "go",
            "test",
            "-mod=readonly",
            "-vet=off",
            "-json",
            "-count=1",
            f"-race={str(race).lower()}",
            "-run=.",
            "-skip=^$",
            "-list=",
            "-bench=",
            "-fuzz=",
            "-short=false",
            "./...",
        ],
        directory,
        env,
    )
    events: list[TestEvent] = [
        json.loads(line) for line in raw.splitlines() if line.strip()
    ]
    tests = verify_events(events, expected, tested)
    print(
        f"{len(expected)} packages; {tests} tests/subtests; "
        f"{len(expected-tested)} build-only packages; zero test skips; race={race}"
    )


def verify_events(
    events: Sequence[TestEvent], expected: set[str], tested: set[str]
) -> int:
    """Distinguish build-only packages from skipped or incomplete actual tests."""
    terminals = [
        event for event in events if event.get("Action") in ("pass", "fail", "skip")
    ]
    packages = [event for event in terminals if not event.get("Test")]
    assertions = [event for event in terminals if event.get("Test")]
    # These independent completeness conditions jointly define passing coverage.
    # pylint: disable=too-many-boolean-expressions
    if (
        not tested
        or not expected
        or len(packages) != len(expected)
        or {event["Package"] for event in packages} != expected
        or any(event["Action"] != "pass" for event in assertions)
        or {event["Package"] for event in assertions} != tested
        or any(
            (
                event["Action"] != "pass"
                if event["Package"] in tested
                else event["Action"] not in ("pass", "skip")
            )
            for event in packages
        )
    ):
        raise ValueError("incomplete module tests or skipped coverage")
    # pylint: enable=too-many-boolean-expressions
    return len(assertions)


def verify_closure(
    directory: Path,
    env: dict[str, str],
    module: ModuleSpec,
    allowed_replacements: Sequence[tuple[str, str | Path]] = (),
) -> None:
    """Reject game/tool imports and all unreviewed module substitutions."""
    metadata: GoMod = json.loads(run(["go", "mod", "edit", "-json"], directory, env))
    actual = {
        (
            item["Old"]["Path"],
            item["New"]["Path"],
            item["Old"].get("Version", ""),
            item["New"].get("Version", ""),
        )
        for item in metadata.get("Replace", [])
    }
    expected = {(name, str(path), "", "") for name, path in allowed_replacements}
    if actual != expected or len(actual) != len(metadata.get("Replace", [])):
        raise ValueError("unexpected local replacement in isolated module")
    dependencies = objects(
        run(
            ["go", "list", "-mod=readonly", "-deps", "-test", "-json", "./..."],
            directory,
            env,
        )
    )
    for item in dependencies:
        owner = item.get("Module", {})
        if (
            "github.com/bradhannah/Ultima" in item["ImportPath"]
            or module["id"].startswith("modules/")
            and (
                item["ImportPath"].startswith(INVENTORY.REPOSITORY + "tools/")
                or item["ImportPath"].startswith("github.com/charmbracelet/")
            )
        ):
            raise ValueError(
                "SDK dependency closure contains game or runtime-to-tool code"
            )
        replacement = owner.get("Replace")
        if replacement and (owner["Path"], replacement["Path"], "", "") not in expected:
            raise ValueError(
                "SDK dependency closure contains an unreviewed replacement"
            )


def prepare_consumer(
    source: Path,
    directory: Path,
    module: ModuleSpec,
    env: dict[str, str],
    replacements: Sequence[tuple[str, str | Path]] = (),
) -> None:
    """Copy an independent consumer and resolve its reviewed test import closure."""
    shutil.copytree(source / "testdata/consumer", directory)
    metadata: GoMod = json.loads(run(["go", "mod", "edit", "-json"], directory, env))
    expected = [
        {"Old": {"Path": INVENTORY.import_path(module)}, "New": {"Path": "../library"}}
    ]
    if metadata.get("Replace") != expected:
        raise ValueError(
            "consumer must have only the reviewed isolated-module substitution"
        )
    for name, destination in replacements:
        run(["go", "mod", "edit", f"-replace={name}={destination}"], directory, env)
    # Consumer fixtures can contain tests only. Resolve their actual test closure
    # before freezing metadata, so newly imported authoring dependencies receive sums.
    run(["go", "list", "-mod=mod", "-deps", "-test", "./..."], directory, env)


def execute_lane(
    root: Path,
    module: ModuleSpec,
    lane: str,
    env: dict[str, str],
    candidate_dependencies: Sequence[ModuleSpec] = (),
) -> None:
    """Execute the chosen Go lane in isolated source copies."""
    source = root / module["id"]
    with tempfile.TemporaryDirectory(prefix="berad-sdk-") as temporary:
        library = Path(temporary) / "library"
        copy_module(root, module, library)
        replacements: list[tuple[str, Path]] = []
        for dependency in candidate_dependencies:
            destination = Path(temporary) / dependency["id"]
            copy_module(root, dependency, destination)
            replacements.append((INVENTORY.import_path(dependency), destination))
        for directory in (library, *(destination for _, destination in replacements)):
            for name, destination in replacements:
                run(
                    ["go", "mod", "edit", f"-replace={name}={destination}"],
                    directory,
                    env,
                )
        if replacements:
            run(["go", "list", "-mod=mod", "./..."], library, env)
        verify_closure(library, env, module, replacements)
        if lane in ("consumer", "candidate"):
            consumer = Path(temporary) / "consumer"
            prepare_consumer(source, consumer, module, env, replacements)
            verify_closure(
                consumer,
                env,
                module,
                [(INVENTORY.import_path(module), "../library"), *replacements],
            )
            run(["go", "vet", "-mod=readonly", "./..."], consumer, env)
            test_packages(consumer, env)
            test_packages(consumer, env, race=True)
            if lane == "candidate":
                test_packages(library, env)
                test_packages(library, env, race=True)
                run(["go", "vet", "-mod=readonly", "./..."], library, env)
        else:
            test_packages(library, env, race=lane == "race")
            if lane == "unit":
                run(["go", "vet", "-mod=readonly", "./..."], library, env)
                run(["go", "build", "-mod=readonly", "./..."], library, env)


def validate(
    root: Path,
    modules: Sequence[ModuleSpec],
    policy: Callable[[], str],
    plan: bool = False,
    selectors: Sequence[str] = (),
) -> None:
    """Plan or run each selected registered module lane."""
    jobs = INVENTORY.selected_jobs(modules, selectors)
    if not modules:
        return
    env = environment()
    cache = root / ".cache/validation/modules"
    for module, lane, dependencies in jobs:
        mode = (
            "candidate dependencies"
            if lane == "candidate"
            else "published dependency pins"
        )
        print(f"scope: {module['id']} / {lane} / {mode}", flush=True)
        if module.get("execution") == "native-render":
            print(
                "execution: native-render "
                "(running Ebitengine test harness; desktop required)",
                flush=True,
            )

        def identity(
            module: ModuleSpec = module,
            lane: str = lane,
            dependencies: Sequence[ModuleSpec] = dependencies,
        ) -> str:
            return fingerprint(
                root, module, lane, policy(), tools(root, env), dependencies
            )

        def execute(
            module: ModuleSpec = module,
            lane: str = lane,
            dependencies: Sequence[ModuleSpec] = dependencies,
        ) -> None:
            execute_lane(root, module, lane, env, dependencies)

        cached_lane(
            cache, module["id"].replace("/", "-") + "-" + lane, identity, execute, plan
        )


def provision(
    root: Path, modules: Sequence[ModuleSpec], selectors: Sequence[str] = ()
) -> None:
    """Explicitly fetch Go dependencies without creating validation evidence."""
    selected = {
        item["id"] for item, _, _ in INVENTORY.selected_jobs(modules, selectors)
    }
    env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="local")
    for module in modules:
        if module["id"] not in selected:
            continue
        source = root / module["id"]
        print(
            f"provision: {module['id']} (network-enabled, no validation proof)",
            flush=True,
        )
        run(["go", "mod", "download"], source, env)
        # Lazy module download can add only go.mod sums. Resolve actual production
        # and test imports here so subsequent readonly/offline lanes have zip sums.
        run(["go", "list", "-mod=mod", "-deps", "-test", "./..."], source, env)
        with tempfile.TemporaryDirectory(prefix="berad-provision-") as temporary:
            copy_module(root, module, Path(temporary) / "library")
            prepare_consumer(source, Path(temporary) / "consumer", module, env)


def copy_module(root: Path, module: ModuleSpec, destination: Path) -> None:
    """Copy authored module inputs, excluding only root generated executables."""
    source = root / module["id"]

    def ignored(directory: str, names: list[str]) -> list[str]:
        return [
            name
            for name in names
            if INVENTORY.tool_output((Path(directory) / name).relative_to(root))
        ]

    shutil.copytree(source, destination, ignore=ignored)
