"""Strict runtime/tools module registration and candidate-dependency selection."""

from collections.abc import Sequence
from pathlib import Path
import re
from typing import NotRequired, TypedDict, cast


class ModuleSpec(TypedDict):
    """Validated namespaced SDK module and its declared validation graph."""

    id: str
    checks: list[str]
    requires: list[str]
    execution: NotRequired[str]


BASE_LANES: list[str] = ["unit", "race", "consumer"]
REPOSITORY = "github.com/bradhannah/berad-sdk/"


def import_path(module: ModuleSpec) -> str:
    """Return the canonical public Go import path."""
    return REPOSITORY + module["id"]


def tool_output(relative: Path) -> bool:
    """Only the tool-module root bin is generated; internal/bin remains source."""
    parts = relative.parts
    return (
        len(parts) >= 3
        and parts[0] == "tools"
        and re.fullmatch(r"berad-[a-z]+", parts[1]) is not None
        and parts[2] == "bin"
    )


def normalize(manifest: object) -> list[ModuleSpec]:
    """Validate an untrusted JSON inventory before exposing typed module records."""
    if manifest == {"schema_version": 1, "stage": "foundation", "modules": []}:
        return []
    if (
        not isinstance(manifest, dict)
        or set(manifest) != {"schema_version", "stage", "modules"}
        or manifest["stage"] != "runtime"
        or not isinstance(manifest["modules"], list)
        or not manifest["modules"]
    ):
        raise ValueError(
            "register module validation before changing the module inventory"
        )
    result: list[ModuleSpec] = []
    for item in manifest["modules"]:
        if manifest["schema_version"] == 1:
            if (
                not isinstance(item, dict)
                or set(item) != {"name", "checks"}
                or not isinstance(item["name"], str)
                or not re.fullmatch(r"berad-[a-z]+", item["name"])
                or item["checks"] != BASE_LANES
            ):
                raise ValueError("register module validation with explicit lanes")
            item = {
                "id": "modules/" + item["name"],
                "checks": item["checks"],
                "requires": [],
            }
        elif manifest["schema_version"] != 2:
            raise ValueError("unsupported SDK inventory schema")
        # Keep schema guards together: later accesses rely on the earlier checks.
        # pylint: disable=too-many-boolean-expressions
        if (
            not isinstance(item, dict)
            or set(item)
            not in (
                {"id", "checks", "requires"},
                {"id", "checks", "requires", "execution"},
            )
            or item.get("execution", "ordinary") not in ("ordinary", "native-render")
            or not isinstance(item["id"], str)
            or not re.fullmatch(r"(?:modules|tools)/berad-[a-z]+", item["id"])
            or not isinstance(item["requires"], list)
            or any(not isinstance(value, str) for value in item["requires"])
            or len(set(item["requires"])) != len(item["requires"])
            or item["checks"]
            != BASE_LANES + (["candidate"] if item["requires"] else [])
        ):
            raise ValueError(
                "register module validation with a canonical id, "
                "dependencies and explicit lanes"
            )
        result.append(cast(ModuleSpec, dict(item)))
        # pylint: enable=too-many-boolean-expressions
    by_id = {item["id"]: item for item in result}
    if len(by_id) != len(result):
        raise ValueError("duplicate SDK module identity")
    for item in result:
        for dependency in item["requires"]:
            if dependency not in by_id or dependency.startswith("tools/"):
                raise ValueError(
                    "SDK dependencies must identify registered runtime modules"
                )
    for item in result:
        dependency_closure(item, result)
    return result


def dependency_closure(
    module: ModuleSpec, modules: Sequence[ModuleSpec], visiting: tuple[str, ...] = ()
) -> list[ModuleSpec]:
    """Resolve the transitive runtime closure while rejecting cycles."""
    identity = module["id"]
    if identity in visiting:
        raise ValueError("cyclic SDK module dependencies")
    by_id = {item["id"]: item for item in modules}
    result: dict[str, ModuleSpec] = {}
    for name in module["requires"]:
        dependency = by_id[name]
        result[name] = dependency
        result.update(
            (item["id"], item)
            for item in dependency_closure(dependency, modules, (*visiting, identity))
        )
    return [result[name] for name in sorted(result)]


def selected_jobs(
    modules: Sequence[ModuleSpec], selectors: Sequence[str]
) -> list[tuple[ModuleSpec, str, list[ModuleSpec]]]:
    """Select requested lanes and downstream candidate integration lanes."""
    known = {item["id"] for item in modules}
    selected = set(selectors or known)
    if not selected <= known:
        raise ValueError(
            "unknown SDK module selector: " + ", ".join(sorted(selected - known))
        )
    jobs: list[tuple[ModuleSpec, str, list[ModuleSpec]]] = []
    for item in modules:
        dependencies = dependency_closure(item, modules)
        for lane in item["checks"]:
            if (
                item["id"] in selected
                or lane == "candidate"
                and selected.intersection(d["id"] for d in dependencies)
            ):
                jobs.append((item, lane, dependencies if lane == "candidate" else []))
    return jobs
