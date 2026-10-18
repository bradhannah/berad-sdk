"""Strict runtime/tools module registration and candidate-dependency selection."""

import re

BASE_LANES = ["unit", "race", "consumer"]
REPOSITORY = "github.com/bradhannah/berad-sdk/"


def import_path(module):
    return REPOSITORY + module["id"]


def normalize(manifest):
    if manifest == {"schema_version": 1, "stage": "foundation", "modules": []}:
        return []
    if (not isinstance(manifest, dict) or set(manifest) != {"schema_version", "stage", "modules"}
            or manifest["stage"] != "runtime" or not isinstance(manifest["modules"], list)
            or not manifest["modules"]):
        raise ValueError("register module validation before changing the module inventory")
    result = []
    for item in manifest["modules"]:
        if manifest["schema_version"] == 1:
            if (not isinstance(item, dict) or set(item) != {"name", "checks"}
                    or not isinstance(item["name"], str) or not re.fullmatch(r"berad-[a-z]+", item["name"])
                    or item["checks"] != BASE_LANES):
                raise ValueError("register module validation with explicit lanes")
            item = {"id": "modules/" + item["name"], "checks": item["checks"], "requires": []}
        elif manifest["schema_version"] != 2:
            raise ValueError("unsupported SDK inventory schema")
        if (not isinstance(item, dict) or set(item) != {"id", "checks", "requires"}
                or not isinstance(item["id"], str) or not re.fullmatch(r"(?:modules|tools)/berad-[a-z]+", item["id"])
                or not isinstance(item["requires"], list)
                or any(not isinstance(value, str) for value in item["requires"])
                or len(set(item["requires"])) != len(item["requires"])
                or item["checks"] != BASE_LANES + (["candidate"] if item["requires"] else [])):
            raise ValueError("register module validation with a canonical id, dependencies and explicit lanes")
        result.append(dict(item))
    by_id = {item["id"]: item for item in result}
    if len(by_id) != len(result):
        raise ValueError("duplicate SDK module identity")
    for item in result:
        for dependency in item["requires"]:
            if dependency not in by_id or dependency.startswith("tools/"):
                raise ValueError("SDK dependencies must identify registered runtime modules")
    for item in result:
        dependency_closure(item, result)
    return result


def dependency_closure(module, modules, visiting=()):
    identity = module["id"]
    if identity in visiting:
        raise ValueError("cyclic SDK module dependencies")
    by_id = {item["id"]: item for item in modules}
    result = {}
    for name in module["requires"]:
        dependency = by_id[name]
        result[name] = dependency
        result.update((item["id"], item) for item in dependency_closure(dependency, modules, (*visiting, identity)))
    return [result[name] for name in sorted(result)]


def selected_jobs(modules, selectors):
    known = {item["id"] for item in modules}
    selected = set(selectors or known)
    if not selected <= known:
        raise ValueError("unknown SDK module selector: " + ", ".join(sorted(selected - known)))
    jobs = []
    for item in modules:
        dependencies = dependency_closure(item, modules)
        for lane in item["checks"]:
            if item["id"] in selected or lane == "candidate" and selected.intersection(d["id"] for d in dependencies):
                jobs.append((item, lane, dependencies if lane == "candidate" else []))
    return jobs
