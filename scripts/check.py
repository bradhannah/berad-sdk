#!/usr/bin/env python3
"""Registered SDK checks and explicit, separately requested network provisioning."""

import argparse
import importlib.util
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
import sysconfig
import tempfile
import time
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("module_checks", ROOT / "scripts/module_checks.py")
MODULE_CHECKS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE_CHECKS)
IGNORED = {".git", ".cache", "__pycache__"}
REQUIRED = (
    "README.md", "LICENSE", "AGENTS.md", "CONTRIBUTING.md", "Makefile", "sdk.json",
    "modules/README.md", "docs/README.md", "docs/SDK_CONVENTIONS.md",
    "docs/API_DESIGN.md", "docs/COMPATIBILITY.md", "docs/VALIDATION.md",
    "docs/LICENSING.md", "docs/EXTRACTION_GUIDE.md",
    ".opencode/skill/berad-sdk-development/SKILL.md",
)
LINK = re.compile(r"\[[^\]\n]*\]\(([^)\n]+)\)")
FENCE = re.compile(r"^```[^\n]*\n.*?^```\s*$", re.MULTILINE | re.DOTALL)


def opencode_output(relative):
    """OpenCode bootstraps these local dependencies when discovering public skills."""
    parts = relative.parts
    return (len(parts) >= 2 and parts[:2] == (".opencode", "node_modules") or
            relative.as_posix() in {".opencode/package.json", ".opencode/package-lock.json",
                                    ".opencode/bun.lock", ".opencode/.gitignore"})


def files(root):
    result = []
    for current, directories, names in os.walk(root):
        directories[:] = sorted(name for name in directories if name not in IGNORED and
                                not opencode_output((Path(current) / name).relative_to(root)) and
                                not MODULE_CHECKS.INVENTORY.tool_output((Path(current) / name).relative_to(root)))
        for name in directories + names:
            path = Path(current) / name
            if opencode_output(path.relative_to(root)):
                continue
            if path.is_symlink():
                raise ValueError(f"source symlink must be an explicit reviewed input: {path}")
        for name in sorted(names):
            path = Path(current) / name
            if opencode_output(path.relative_to(root)):
                continue
            if not path.is_file():
                raise ValueError(f"unsupported source input: {path}")
            result.append(path)
    return sorted(result)


def contracts(root):
    paths = files(root)
    for name in REQUIRED:
        if not (root / name).is_file():
            raise ValueError(f"missing required file: {name}")
    manifest = json.loads((root / "sdk.json").read_text())
    modules = MODULE_CHECKS.inventory(root, manifest, paths)
    markdown = [path for path in paths if path.suffix == ".md"]
    for path in markdown:
        text = path.read_text(encoding="utf-8")
        if re.search(r"(?:/Users/|/home/)[A-Za-z0-9_.-]+/", text):
            raise ValueError(f"personal source path in {path.relative_to(root)}")
        for destination in LINK.findall(FENCE.sub("", text)):
            parsed = urlsplit(destination.split(' "', 1)[0])
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            target = (path.parent / unquote(parsed.path)).resolve()
            if not target.is_relative_to(root.resolve()) or not target.exists():
                raise ValueError(f"broken or external local link: {path.relative_to(root)} -> {destination}")
    owners = [root] + [root / module["id"] for module in modules]
    for owner in owners:
        docs = owner / "docs"
        if not docs.is_dir():
            continue
        catalog = (docs / "README.md").read_text()
        destinations = [unquote(urlsplit(link).path) for link in LINK.findall(catalog)]
        for path in docs.rglob("*.md"):
            relative = path.relative_to(docs).as_posix()
            if destinations.count(relative) != 1:
                raise ValueError(f"document must be cataloged exactly once: {path.relative_to(root)}")
    names = set()
    for path in sorted((root / ".opencode/skill").rglob("SKILL.md")):
        text = path.read_text()
        identity = re.match(r"---\nname: ([a-z0-9-]+)\ndescription: Use when [^\n]+\n---\n", text)
        if identity is None or identity[1] != path.parent.name:
            raise ValueError(f"invalid public SDK skill identity/description: {path.relative_to(root)}")
        if identity[1] in names:
            raise ValueError(f"duplicate public SDK skill: {identity[1]}")
        names.add(identity[1])
    if "berad-sdk-development" not in names:
        raise ValueError("public SDK foundation skill missing")
    if "MIT License" not in (root / "LICENSE").read_text():
        raise ValueError("missing MIT license")
    return modules


def fingerprint(root):
    """Bind regression inputs, interpreter/runtime sources, and execution context."""
    hasher = hashlib.sha256()
    paths = [p for p in files(root) if p.relative_to(root).parts[0] in {"scripts", "tests"}
             or p.relative_to(root).as_posix() in {"sdk.json", "Makefile"}]
    for path in paths:
        hasher.update(str(path.relative_to(root)).encode())
        hasher.update(path.read_bytes())
    executable = Path(sys.executable).resolve()
    hasher.update(str(executable).encode())
    hasher.update(executable.read_bytes())
    stdlib = Path(sysconfig.get_path("stdlib"))
    for current, directories, names in os.walk(stdlib):
        directories[:] = sorted(n for n in directories
                                if n not in {"site-packages", "dist-packages", "__pycache__"})
        for name in sorted(names):
            if name.endswith((".py", ".so", ".pyd", ".dll")):
                path = Path(current) / name
                hasher.update(str(path).encode())
                hasher.update(path.read_bytes())
    environment = {k: v for k, v in os.environ.items()
                   if k not in {"_", "PWD", "OLDPWD", "SHLVL"} and not k.startswith("OPENCODE_")}
    hasher.update(json.dumps([sys.version, platform.platform(), environment], sort_keys=True).encode())
    return hasher.hexdigest()


def reusable(record, identity):
    try:
        return json.loads(record.read_text()) == {"schema": 1, "state": "passed", "identity": identity}
    except (OSError, ValueError):
        return False


def regression_tests(root, cache, identity_function=fingerprint, runner=subprocess.run):
    cache.mkdir(parents=True, exist_ok=True)
    lock = cache / "running"
    try:
        lock.mkdir()
    except FileExistsError as error:
        raise ValueError(f"validation already running; if interrupted, verify no runner remains before removing {lock}") from error
    record = cache / "checker-tests.json"
    try:
        before = identity_function(root)
        if reusable(record, before):
            print("checker-tests: REUSED (material inputs unchanged)")
            return
        record.unlink(missing_ok=True)
        print("checker-tests: RUN (no matching successful result)", flush=True)
        runner([sys.executable, "-I", "-B", "-m", "unittest", "discover", "-s", "tests", "-v"],
               cwd=root, check=True)
        if before != identity_function(root):
            raise ValueError("regression inputs changed during execution; result is not reusable")
        with tempfile.NamedTemporaryFile(mode="w", dir=cache, delete=False) as output:
            json.dump({"schema": 1, "state": "passed", "identity": before}, output)
            temporary = Path(output.name)
        os.replace(temporary, record)
    finally:
        lock.rmdir()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("plan", "validate", "quality-gate", "provision"))
    parser.add_argument("--module", action="append", default=[], help="registered identity, e.g. modules/berad-audio")
    args = parser.parse_args()
    start = time.monotonic()
    try:
        modules = contracts(ROOT)
        print("contracts: PASS (cheap manifest, inventory, links, catalog, skill and source-path checks)")
        MODULE_CHECKS.INVENTORY.selected_jobs(modules, args.module)
        if args.operation == "provision":
            MODULE_CHECKS.provision(ROOT, modules, args.module)
            return 0
        cache = ROOT / ".cache/validation"
        if args.operation == "plan":
            action = "REUSE" if reusable(cache / "checker-tests.json", fingerprint(ROOT)) else "RUN"
            print(f"checker-tests: {action} (matching material-input evidence required)")
            MODULE_CHECKS.validate(ROOT, modules, lambda: fingerprint(ROOT), plan=True, selectors=args.module)
            if not modules:
                print("runtime/native/consumer suites: NOT REGISTERED (foundation stage)")
        else:
            regression_tests(ROOT, cache)
            MODULE_CHECKS.validate(ROOT, modules, lambda: fingerprint(ROOT), selectors=args.module)
            scope = ",".join(args.module) or "all registered modules"
            print(f"SDK {args.operation}: PASS ({scope}); elapsed={time.monotonic() - start:.2f}s")
        return 0
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"Foundation check failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
