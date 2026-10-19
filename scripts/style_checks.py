"""Pinned, offline formatting and full-module lint with material-input reuse."""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

PINS = {
    "goimports": ("golang.org/x/tools", "golang.org/x/tools/cmd/goimports", "v0.37.0"),
    "golangci-lint": ("github.com/golangci/golangci-lint/v2",
                      "github.com/golangci/golangci-lint/v2/cmd/golangci-lint", "v2.12.2"),
}


def executable(root, name):
    return root / ".cache/tools" / (name + (".exe" if os.name == "nt" else ""))


def provision(root, checks):
    directory = root / ".cache/tools"
    directory.mkdir(parents=True, exist_ok=True)
    environment = dict(os.environ, GOBIN=str(directory), GOWORK="off", GOTOOLCHAIN="local")
    for name, (_, package, version) in PINS.items():
        print(f"provision tool: {name}@{version}", flush=True)
        checks.run(["go", "install", package + "@" + version], root, environment)


def tool_identity(root, checks, environment):
    digest = hashlib.sha256()
    for name, (module, _, version) in PINS.items():
        binary = executable(root, name)
        if not binary.is_file():
            raise ValueError(f"missing pinned {name}; run make tools")
        metadata = checks.run(["go", "version", "-m", str(binary)], root, environment)
        expected = ["mod", module, version]
        if not any(line.split()[:3] == expected for line in metadata.splitlines()):
            raise ValueError(f"{name} must be built from {module}@{version}; run make tools")
        if name == "golangci-lint":
            reported = json.loads(checks.run([str(binary), "version", "--json"], root, environment))
            if reported.get("version", reported.get("Version")) != version.removeprefix("v"):
                raise ValueError("golangci-lint version does not match its required pin")
        digest.update(str(binary.resolve()).encode())
        digest.update(binary.read_bytes())
        digest.update(metadata.encode())
    return digest.hexdigest()


def source_files(root, modules, checks):
    return sorted(path for module in modules for path in (root / module["id"]).rglob("*.go")
                  if not checks.INVENTORY.tool_output(path.relative_to(root)))


def formatting(root, modules, checks, environment, write=False):
    paths = source_files(root, modules, checks)
    if not paths:
        raise ValueError("format inventory must contain Go source")
    findings = []
    for offset in range(0, len(paths), 100):
        output = checks.run([str(executable(root, "goimports")), "-w" if write else "-l",
                            *(str(path) for path in paths[offset:offset + 100])], root, environment)
        findings.extend(output.splitlines())
    if findings:
        raise ValueError("Go files require make format:\n" + "\n".join(findings))
    print(f"format{'-check' if not write else ''}: PASS ({len(paths)} authored Go files, including tests/consumers)", flush=True)


def verify_config(root, checks, environment):
    binary, config = executable(root, "golangci-lint"), root / ".golangci.yml"
    checks.run([str(binary), "config", "verify", "--config", str(config)], root, environment)
    # Exercise effective policy, not just YAML spelling. Controls use checked errors.
    fixture = '''package fixture
import "os"
func probe(value any) {
    _, _ = os.Open("missing") // want errcheck
    _ = value.(string) // want errcheck
    file, err := os.Open("missing")
    if err == nil { if err := file.Close(); err != nil { panic(err) } }
    _, ok := value.(string)
    _ = ok
}
'''
    with tempfile.TemporaryDirectory(prefix="sdk-lint-policy-") as directory:
        directory = Path(directory)
        (directory / "go.mod").write_text("module example.com/lintpolicy\n\ngo 1.27.0\n")
        (directory / "fixture.go").write_text(fixture)
        output = checks.run([str(binary), "run", "--config", str(config),
                             "--issues-exit-code=0", "--show-stats=false", "--output.text.path=", "--output.json.path=stdout",
                             "./..."], directory, environment)
        issues = json.loads(output).get("Issues", [])
        observed = {item["Pos"]["Line"] for item in issues if item["FromLinter"] == "errcheck"}
        wanted = {index for index, line in enumerate(fixture.splitlines(), 1) if "want errcheck" in line}
        if observed != wanted:
            raise ValueError(f"effective errcheck policy failed: expected {wanted}, got {observed}")
    print("lint-config-check: PASS (pinned schema and ignored-error/type-assertion regressions)", flush=True)


def lint_module(root, module, checks, environment, fix=False):
    directory = root / module["id"]
    packages = checks.run(["go", "list", "-mod=readonly", "-f", "{{.Dir}}", "./..."], directory, environment).splitlines()
    if not packages or len(packages) != len(set(packages)):
        raise ValueError("lint package inventory must be nonempty and unique")
    command = [str(executable(root, "golangci-lint")), "run", "--config", str(root / ".golangci.yml"),
               "--new=false", "--new-from-rev=", "--new-from-patch=", "--new-from-merge-base=",
               "--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false",
               "--output.text.path=stdout",
               "--output.json.path", str(root / ".cache" / (module["id"].replace("/", "-") + "-lint.json"))]
    if fix:
        command.append("--fix")
    print(f"lint: {module['id']} ({len(packages)} packages; full source, no revision cutoff)", flush=True)
    print(checks.run([*command, *packages], directory, environment), end="", flush=True)
    if not fix:
        lint_consumer(root, module, checks, environment, command)


def lint_consumer(root, module, checks, environment, command):
    with tempfile.TemporaryDirectory(prefix="sdk-lint-consumer-") as temporary:
        library, consumer = Path(temporary) / "library", Path(temporary) / "consumer"
        checks.copy_module(root, module, library)
        checks.prepare_consumer(root / module["id"], consumer, module, environment)
        checks.verify_closure(consumer, environment, module, [(checks.INVENTORY.import_path(module), "../library")])
        packages = list(checks.objects(checks.run(["go", "list", "-mod=readonly", "-json", "./..."], consumer, environment)))
        if not packages:
            raise ValueError("consumer package inventory must be nonempty")
        directories = [item["Dir"] for item in packages if item.get("GoFiles") or item.get("CgoFiles")]
        if directories:
            # Keep consumer diagnostics separate from the owning module's report.
            command = list(command)
            index = command.index("--output.json.path") + 1
            command[index] = str(Path(command[index]).with_name(module["id"].replace("/", "-") + "-consumer-lint.json"))
            print(checks.run([*command, *directories], consumer, environment), end="", flush=True)
        else:
            print(f"lint: {module['id']} consumer is test-only; format, vet and test/race lanes cover it", flush=True)


def validate(root, modules, checks, policy, operation="check", selectors=(), plan=False):
    if not modules and not selectors:
        print("style: NOT REGISTERED (foundation stage)", flush=True)
        return
    environment = checks.environment()
    selected = [module for module in modules if not selectors or module["id"] in selectors]
    if not selected:
        raise ValueError("style selection must contain registered modules")
    config = root / ".golangci.yml"
    if not config.is_file():
        raise ValueError("missing SDK lint policy")
    # Tool/config/policy changes invalidate style proof; unit/race graphs stay separate.
    def shared():
        return hashlib.sha256((policy() + checks.tools(root, environment) +
                               tool_identity(root, checks, environment)).encode() + config.read_bytes()).hexdigest()
    if operation == "format":
        tool_identity(root, checks, environment)
        formatting(root, selected, checks, environment, write=True)
        return
    cache = root / ".cache/validation/style"
    if operation in ("check", "lint", "lint-fix", "lint-config-check"):
        checks.cached_lane(cache, "config", shared,
                           lambda: verify_config(root, checks, environment), plan)
    if operation == "lint-config-check":
        return
    fix_failures = []
    for module in selected:
        # Consumer source is included even though it is a separate nested module.
        identity = lambda module=module: checks.fingerprint(root, module, "consumer", shared(), "style")
        name = module["id"].replace("/", "-")
        if operation in ("check", "format-check"):
            checks.cached_lane(cache, name + "-format", identity,
                               lambda module=module: formatting(root, [module], checks, environment), plan)
        if operation in ("check", "lint"):
            checks.cached_lane(cache, name + "-lint", identity,
                               lambda module=module: lint_module(root, module, checks, environment), plan)
        if operation == "lint-fix":
            try:
                lint_module(root, module, checks, environment, fix=True)
            except subprocess.CalledProcessError as error:
                if error.returncode != 1:
                    raise
                fix_failures.append(module["id"])
    if fix_failures:
        raise ValueError("automatic fixes left findings requiring review: " + ", ".join(fix_failures))
