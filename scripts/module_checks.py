"""Offline, isolated Go module lanes with source-bound selective result reuse."""

import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tempfile


LANES = ("unit", "race", "consumer")
PREFIX = "github.com/bradhannah/berad-sdk/modules/"


def inventory(root, manifest, paths):
    if manifest == {"schema_version": 1, "stage": "foundation", "modules": []}:
        modules = []
    elif (set(manifest) == {"schema_version", "stage", "modules"} and manifest["schema_version"] == 1
          and manifest["stage"] == "runtime" and isinstance(manifest["modules"], list) and manifest["modules"]):
        modules = manifest["modules"]
    else:
        raise ValueError("register module validation before changing the foundation inventory")
    names = set()
    allowed_mods = set()
    for module in modules:
        if (not isinstance(module, dict) or set(module) != {"name", "checks"}
                or not re.fullmatch(r"berad-[a-z]+", module.get("name", ""))
                or module["checks"] != list(LANES) or module["name"] in names):
            raise ValueError("register module validation with unique names and unit/race/consumer checks")
        name = module["name"]
        names.add(name)
        directory = root / "modules" / name
        for required in ("go.mod", "go.sum", "LICENSE", "README.md", "doc.go", "testdata/consumer/go.mod"):
            if not (directory / required).is_file():
                raise ValueError(f"missing module contract/input: {name}/{required}")
        metadata = (directory / "go.mod").read_text()
        if not re.search(r"^module " + re.escape(PREFIX + name) + r"\s*$", metadata, re.M):
            raise ValueError("module must use its canonical SDK identity")
        if re.search(r"^\s*replace\b", metadata, re.M):
            raise ValueError("published SDK module must not use replacements")
        allowed_mods.update((directory / "go.mod", directory / "testdata/consumer/go.mod"))
    for path in paths:
        if path.name == "go.work" or path.name == "go.mod" and path not in allowed_mods:
            raise ValueError(f"unregistered runtime code/module: {path.relative_to(root)}")
        if path.suffix == ".go":
            relative = path.relative_to(root).parts
            if len(relative) < 3 or relative[0] != "modules" or relative[1] not in names:
                raise ValueError(f"unregistered runtime code/module: {path.relative_to(root)}")
            if "//go:embed" in path.read_text():
                raise ValueError("register embedded asset dependency selection before adding go:embed")
    return modules


def run(command, cwd, env):
    result = subprocess.run(command, cwd=cwd, env=env, check=False, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=600)
    if result.returncode:
        print(result.stdout, end="")
        print(result.stderr, end="")
        raise subprocess.CalledProcessError(result.returncode, command)
    return result.stdout


def objects(text):
    decoder = json.JSONDecoder()
    while text.strip():
        item, end = decoder.raw_decode(text.lstrip())
        yield item
        text = text.lstrip()[end:]


def environment():
    env = dict(os.environ, GOWORK="off", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local")
    if env.get("GOFLAGS"):
        raise ValueError("module checks require empty GOFLAGS; fixed lane flags own test selection")
    return env


def tools(root, env):
    values = json.loads(run(["go", "env", "-json"], root, env))
    if values.get("GOFLAGS"):
        raise ValueError("persisted GOFLAGS override registered module checks")
    values["GOGCCFLAGS"] = re.sub(r"go-build[0-9]+", "go-build<TEMP>", values.get("GOGCCFLAGS", ""))
    digest = hashlib.sha256(json.dumps(values, sort_keys=True).encode())
    digest.update(run(["go", "version"], root, env).encode())
    selected_go = Path(shutil.which("go", path=env.get("PATH"))).resolve(strict=True)
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
            raise ValueError("registered race checks require provisioned C/C++ compilers")
        digest.update(str(Path(path).resolve()).encode())
        digest.update(Path(path).read_bytes())
    return digest.hexdigest()


def fingerprint(root, module, lane, policy, tool_identity):
    digest = hashlib.sha256(json.dumps([module, lane, policy, tool_identity], sort_keys=True).encode())
    directory = root / "modules" / module["name"]
    for path in sorted(directory.rglob("*")):
        if not path.is_file():
            continue
        relative = path.relative_to(directory)
        if relative.parts[:2] == ("testdata", "consumer") and lane != "consumer":
            continue
        if str(relative) in ("README.md", "LICENSE"):
            continue  # Cheap contracts still run on every invocation; no embeds allowed.
        digest.update(str(relative).encode())
        digest.update(str(path.stat().st_mode).encode())
        digest.update(path.read_bytes())
    return digest.hexdigest()


def reusable(record, identity):
    try:
        return json.loads(record.read_text()) == {"schema": 1, "state": "passed", "identity": identity}
    except (ValueError, OSError):
        return False


def cached_lane(cache, name, identity, execute, plan=False):
    record = cache / (name + ".json")
    if plan:
        print(f"{name}: {'REUSE' if reusable(record, identity()) else 'RUN'} (module/dependencies/tools/lane inputs)")
        return
    cache.mkdir(parents=True, exist_ok=True)
    lock = cache / (name + ".running")
    try:
        lock.mkdir()
    except FileExistsError as error:
        raise ValueError(f"{name}: validation already running; inspect interrupted jobs before removing {lock}") from error
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
        with tempfile.NamedTemporaryFile(mode="w", dir=cache, delete=False) as output:
            json.dump({"schema": 1, "state": "passed", "identity": before}, output)
            temporary = Path(output.name)
        os.replace(temporary, record)
    finally:
        lock.rmdir()


def test_packages(directory, env, race=False):
    expected = set(run(["go", "list", "-mod=readonly", "./..."], directory, env).splitlines())
    raw = run(["go", "test", "-mod=readonly", "-vet=off", "-json", "-count=1", f"-race={str(race).lower()}",
               "-run=.", "-skip=^$", "-list=", "-bench=", "-fuzz=", "-short=false", "./..."], directory, env)
    events = [json.loads(line) for line in raw.splitlines() if line.strip()]
    passed = {e["Package"] for e in events if e.get("Action") == "pass" and not e.get("Test")}
    tests = sum(e.get("Action") == "pass" and bool(e.get("Test")) for e in events)
    if not tests or not expected or passed != expected or any(e.get("Action") == "skip" for e in events):
        raise ValueError("incomplete module tests or skipped coverage")
    print(f"{len(passed)} packages; {tests} tests/subtests; zero skips; race={race}")


def execute_lane(root, module, lane, env):
    source = root / "modules" / module["name"]
    with tempfile.TemporaryDirectory(prefix="berad-sdk-") as temporary:
        library = Path(temporary) / "library"
        shutil.copytree(source, library)
        dependencies = objects(run(["go", "list", "-mod=readonly", "-deps", "-test", "-json", "./..."], library, env))
        for item in dependencies:
            owner = item.get("Module", {})
            if owner.get("Replace") or "github.com/bradhannah/Ultima" in item["ImportPath"]:
                raise ValueError("SDK dependency closure contains game code or a local replacement")
        if lane == "consumer":
            consumer = Path(temporary) / "consumer"
            shutil.copytree(source / "testdata/consumer", consumer)
            metadata = json.loads(run(["go", "mod", "edit", "-json"], consumer, env))
            expected = [{"Old": {"Path": PREFIX + module["name"]}, "New": {"Path": "../library"}}]
            if metadata.get("Replace") != expected:
                raise ValueError("consumer must have only the reviewed isolated-module substitution")
            run(["go", "list", "-mod=mod", "./..."], consumer, env)
            test_packages(consumer, env)
            test_packages(consumer, env, race=True)
        else:
            test_packages(library, env, race=lane == "race")
            if lane == "unit":
                run(["go", "vet", "-mod=readonly", "./..."], library, env)
                run(["go", "build", "-mod=readonly", "./..."], library, env)


def validate(root, modules, policy, plan=False):
    if not modules:
        return
    env = environment()
    cache = root / ".cache/validation/modules"
    for module in modules:
        for lane in module["checks"]:
            identity = lambda: fingerprint(root, module, lane, policy(), tools(root, env))
            execute = lambda: execute_lane(root, module, lane, env)
            cached_lane(cache, module["name"] + "-" + lane, identity, execute, plan)
