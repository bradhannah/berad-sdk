#!/usr/bin/env python3
"""Standalone, offline Python quality checks; provisioning is an explicit operation."""

from __future__ import annotations

import argparse
import hashlib
import importlib.metadata
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import sysconfig
from types import ModuleType
import venv

ROOT = Path(__file__).resolve().parents[1]
POLICY = ("pyproject.toml", "requirements-quality.in", "requirements-quality.lock")
OPERATIONS = (
    "python-tools",
    "python-format",
    "python-format-check",
    "python-lint",
    "python-check",
    "python-identity",
    "pre-commit",
    "_tool-identity",
)


def require_python() -> None:
    """Fail before starting checks under an unsupported bootstrap interpreter."""
    if sys.version_info < (3, 12):
        raise ValueError(
            "Python 3.12+ required; select PYTHON=python3.12 "
            "or provision .cache/python-quality"
        )


def load_module(name: str, path: Path) -> ModuleType:
    """Load an explicit repository module, without ambient import-path discovery."""
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise ValueError(f"cannot load repository module: {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def git(root: Path, *arguments: str) -> bytes:
    """Run a bounded Git inspection in the selected repository."""
    return subprocess.run(
        ["git", "-C", str(root), *arguments],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=30,
    ).stdout


def source_files(root: Path) -> list[Path]:
    """Include tracked and nonignored new Python, rejecting source symlinks."""
    names = git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
    paths: list[Path] = []
    for raw in sorted(set(names.split(b"\0")) - {b""}):
        relative = Path(os.fsdecode(raw))
        if any(part in {".cache", "__pycache__"} for part in relative.parts):
            continue
        path = root / relative
        if any(
            parent.is_symlink()
            for parent in (path, *path.parents)
            if parent.is_relative_to(root) and parent != root
        ):
            raise ValueError(f"Python source symlink is not supported: {relative}")
        if relative.suffix != ".py":
            continue
        if not path.exists():
            continue  # Tracked deletion: absent from the candidate worktree.
        if not path.is_file():
            raise ValueError(f"Python source must be a regular file: {relative}")
        paths.append(path)
    if not paths:
        raise ValueError("Python source inventory is empty")
    return paths


def interpreter(root: Path) -> Path:
    """Select only the explicitly provisioned repository tool environment."""
    for directory in (root / ".cache", root / ".cache/python-quality"):
        if directory.is_symlink():
            raise ValueError(f"refusing symlinked tool environment: {directory}")
    path = (
        root
        / ".cache/python-quality"
        / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
    )
    if not path.is_file():
        raise ValueError(
            "missing Python quality tools; run make python-tools "
            "PYTHON_BOOTSTRAP=python3.12"
        )
    return path


def provision(root: Path) -> None:
    """Create the development venv and install the fully hashed dependency lock."""
    require_python()
    directory = root / ".cache/python-quality"
    for path in (root / ".cache", directory):
        if path.is_symlink():
            raise ValueError(f"refusing symlinked tool environment: {path}")
    if directory.exists():
        raise ValueError(
            "tool environment already exists; remove .cache/python-quality "
            "deliberately before reprovisioning the lock"
        )
    lock = root / "requirements-quality.lock"
    if lock.is_symlink() or not lock.is_file():
        raise ValueError("quality lock must be a regular, non-symlink file")
    venv.EnvBuilder(with_pip=True).create(directory)
    subprocess.run(
        [
            str(interpreter(root)),
            "-I",
            "-B",
            "-m",
            "pip",
            "install",
            "--require-hashes",
            "--only-binary=:all:",
            "--disable-pip-version-check",
            "-r",
            str(root / "requirements-quality.lock"),
        ],
        cwd=root,
        check=True,
    )


def runtime_identity() -> str:
    """Bind interpreter implementation and standard-library source/native modules."""
    digest = hashlib.sha256(sys.version.encode())
    executable = Path(sys.executable).resolve(strict=True)
    digest.update(str(executable).encode())
    digest.update(executable.read_bytes())
    for current, directories, names in os.walk(sysconfig.get_path("stdlib")):
        directories[:] = sorted(
            name
            for name in directories
            if name not in {"site-packages", "dist-packages", "__pycache__"}
        )
        for name in sorted(names):
            if name.endswith((".py", ".so", ".pyd", ".dll")):
                path = Path(current) / name
                digest.update(str(path).encode())
                digest.update(path.read_bytes())
    return digest.hexdigest()


def installed_library_identity() -> str:
    """Include added import files and bytecode, not only wheel RECORD entries."""
    digest = hashlib.sha256()
    for directory in sorted(
        {sysconfig.get_path("purelib"), sysconfig.get_path("platlib")}
    ):
        for path in sorted(Path(directory).rglob("*")):
            if path.is_symlink():
                raise ValueError(f"unsupported installed tool symlink: {path}")
            if path.is_dir():
                continue
            if not path.is_file():
                raise ValueError(f"unsupported installed tool input: {path}")
            digest.update(str(path).encode())
            digest.update(str(path.stat().st_mode).encode())
            digest.update(path.read_bytes())
    return digest.hexdigest()


def installed_identity(root: Path) -> str:
    """Hash actual installed tool/dependency bytes, never trusting RECORD hashes."""
    require_python()
    digest = hashlib.sha256(
        (runtime_identity() + installed_library_identity()).encode()
    )
    pins = re.findall(
        r"^([a-z0-9-]+)==([^\s;]+)([^\n]*)",
        (root / "requirements-quality.lock").read_text(encoding="utf-8"),
        re.MULTILINE,
    )
    if not pins:
        raise ValueError("quality lock has no pinned dependencies")
    for name, version, marker in pins:
        if "sys_platform == 'win32'" in marker and sys.platform != "win32":
            continue
        distribution = importlib.metadata.distribution(name)
        if distribution.version != version:
            raise ValueError(f"{name} must be {version}; run make python-tools")
        digest.update(f"{name}=={version}".encode())
        if distribution.files is None:
            raise ValueError(f"missing installed file inventory: {name}")
        for relative in sorted(distribution.files):
            path = Path(distribution.locate_file(relative))
            if path.is_symlink() or not path.is_file():
                raise ValueError(f"unsupported installed tool input: {path}")
            digest.update(str(path).encode())
            digest.update(str(path.stat().st_mode).encode())
            digest.update(path.read_bytes())
    return digest.hexdigest()


def tool_identity(root: Path) -> str:
    """Inspect dependency implementation in the exact interpreter used for checks."""
    result = subprocess.run(
        [
            str(interpreter(root)),
            "-I",
            "-B",
            str(ROOT / "scripts/python_quality.py"),
            "_tool-identity",
            "--root",
            str(root),
        ],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=120,
    )
    return result.stdout.strip()


def fingerprint(root: Path) -> str:
    """Bind complete source inventory, policy, runtime and actual tool dependencies."""
    digest = hashlib.sha256(tool_identity(root).encode())
    for path in [
        *source_files(root),
        *(root / name for name in POLICY),
        root / ".githooks/pre-commit",
    ]:
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"missing or nonregular Python policy input: {path}")
        digest.update(str(path.relative_to(root)).encode())
        digest.update(str(path.stat().st_mode).encode())
        digest.update(path.read_bytes())
    environment = {
        key: value
        for key, value in os.environ.items()
        if key not in {"_", "PWD", "OLDPWD", "SHLVL"}
        and not key.startswith("OPENCODE_")
    }
    digest.update(json.dumps(environment, sort_keys=True).encode())
    return digest.hexdigest()


def run_tool(root: Path, tool: str, paths: list[Path], *, write: bool = False) -> None:
    """Run the pinned checker with repository policy and no user configuration."""
    command = [str(interpreter(root)), "-I", "-B", "-m", tool]
    config = str(root / "pyproject.toml")
    if tool == "black":
        command += ["--config", config]
        if not write:
            command += ["--check", "--diff"]
    elif tool == "pylint":
        command += [
            "--rcfile",
            config,
            "--score=no",
            "--persistent=no",
            "--reports=no",
            "--output-format=json",
        ]
        command += ["--", *(str(path) for path in paths)]
        result = subprocess.run(
            command, cwd=root, check=False, capture_output=True, text=True, timeout=300
        )
        if result.stderr:
            print(result.stderr, end="", file=sys.stderr)
        try:
            messages: object = json.loads(result.stdout)
        except ValueError as error:
            raise ValueError(
                "Pylint did not produce a valid diagnostic payload"
            ) from error
        if not isinstance(messages, list):
            raise ValueError("Pylint diagnostic payload must be a list")
        if messages or result.returncode:
            print(result.stdout, end="", file=sys.stderr)
            raise subprocess.CalledProcessError(result.returncode or 1, command)
        return
    else:
        raise ValueError(f"unsupported Python quality tool: {tool}")
    subprocess.run(
        [*command, *(str(path) for path in paths)], cwd=root, check=True, timeout=300
    )


def validate(
    root: Path, operation: str = "python-check", *, plan: bool = False
) -> None:
    """Use the SDK lane cache with repository-wide Python selection."""
    cache = root / ".cache/validation"
    try:
        require_python()
        checks = load_module("python_quality_lanes", ROOT / "scripts/module_checks.py")
    except BaseException:
        if not plan:
            for name in ("python-format-check", "python-lint"):
                if operation in {"python-check", "python-format", name}:
                    (cache / f"{name}.json").unlink(missing_ok=True)
        raise
    if operation == "python-format":
        for name in ("python-format-check", "python-lint"):
            (cache / f"{name}.json").unlink(missing_ok=True)
        tool_identity(root)
        run_tool(root, "black", source_files(root), write=True)
        return
    lanes = {"python-format-check": "black", "python-lint": "pylint"}
    if operation not in {*lanes, "python-check"}:
        raise ValueError(f"unknown Python quality operation: {operation}")
    for name, tool in lanes.items():
        if operation in {"python-check", name}:
            checks.cached_lane(
                cache,
                name,
                lambda: fingerprint(root),
                lambda tool=tool: run_tool(root, tool, source_files(root)),
                plan,
            )


def candidate_identity(root: Path) -> bytes:
    """Require a staged candidate matching the validated worktree bytes."""
    if git(root, "diff", "--name-only", "-z") or git(
        root, "ls-files", "--others", "--exclude-standard", "-z"
    ):
        raise ValueError(
            "pre-commit requires an index matching the worktree; "
            "stage or set aside unstaged/new files"
        )
    return git(root, "ls-files", "--stage", "-z")


def pre_commit(root: Path) -> None:
    """Check only, rejecting candidate changes before or during validation."""
    before = candidate_identity(root)
    validate(root)
    if before != candidate_identity(root):
        raise ValueError("staged candidate changed during Python checks")


def main() -> int:
    """Run independent Python operations without invoking Go/native tooling."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=OPERATIONS)
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args()
    try:
        if args.operation == "python-tools":
            provision(args.root)
        elif args.operation == "_tool-identity":
            print(installed_identity(args.root))
        elif args.operation == "python-identity":
            print(fingerprint(args.root))
        elif args.operation == "pre-commit":
            pre_commit(args.root)
        else:
            validate(args.root, args.operation)
        return 0
    except (
        OSError,
        ValueError,
        subprocess.SubprocessError,
        importlib.metadata.PackageNotFoundError,
    ) as error:
        print(f"Python quality failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
