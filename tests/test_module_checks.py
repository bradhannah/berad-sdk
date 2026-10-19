"""Selection, isolation and failure-boundary proof for registered Go lanes."""

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("module_checks", ROOT / "scripts/module_checks.py")
checks = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checks)


class ModuleChecksTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.module = {"id": "modules/berad-audio", "checks": list(checks.INVENTORY.BASE_LANES), "requires": []}
        self.directory = self.root / "modules/berad-audio"
        (self.directory / "testdata/consumer").mkdir(parents=True)
        (self.directory / "runtime.go").write_text("package audio\n")
        (self.directory / "README.md").write_text("# Audio\n")
        (self.directory / "testdata/consumer/consumer_test.go").write_text("package consumer_test\n")

    def identity(self, lane="unit", policy="policy", tools="tools"):
        return checks.fingerprint(self.root, self.module, lane, policy, tools)

    def test_tool_output_is_not_an_input_or_isolated_source_but_internal_bin_is(self):
        module = {"id": "tools/berad-audio", "checks": list(checks.INVENTORY.BASE_LANES), "requires": []}
        source = self.root / module["id"]
        (source / "bin").mkdir(parents=True)
        (source / "internal/bin").mkdir(parents=True)
        code = source / "internal/bin/source.go"
        code.write_text("package bin\n")
        identity = lambda: checks.fingerprint(self.root, module, "unit", "policy", "tools")
        before = identity()
        (source / "bin/audio-editor").write_bytes(b"generated executable")
        self.assertEqual(before, identity())
        copied = self.root / "isolated"
        checks.copy_module(self.root, module, copied)
        self.assertFalse((copied / "bin").exists())
        self.assertEqual((copied / "internal/bin/source.go").read_text(), code.read_text())
        code.write_text("package bin\n// changed source\n")
        self.assertNotEqual(before, identity())
        self.assertFalse(checks.INVENTORY.tool_output(Path("modules/berad-audio/bin/source.go")))

    def test_prose_and_unrelated_module_preserve_unit_proof_but_dependencies_invalidate(self):
        before = self.identity()
        (self.directory / "README.md").write_text("# Clarified audio contract\n")
        other = self.root / "modules/berad-text"
        other.mkdir()
        (other / "layout.go").write_text("package text\n")
        self.assertEqual(before, self.identity())
        (self.directory / "go.sum").write_text("changed dependency\n")
        self.assertNotEqual(before, self.identity())
        self.assertNotEqual(self.identity(), self.identity(tools="new-toolchain"))
        self.assertNotEqual(self.identity(), self.identity(policy="new-policy"))

    def test_consumer_changes_do_not_invalidate_unit_but_sdk_changes_invalidate_both(self):
        unit, consumer = self.identity(), self.identity("consumer")
        path = self.directory / "testdata/consumer/consumer_test.go"
        path.write_text(path.read_text() + "// changed consumer\n")
        self.assertEqual(unit, self.identity())
        self.assertNotEqual(consumer, self.identity("consumer"))
        consumer = self.identity("consumer")
        (self.directory / "runtime.go").write_text("package audio\n// changed runtime\n")
        self.assertNotEqual(unit, self.identity())
        self.assertNotEqual(consumer, self.identity("consumer"))
        self.assertNotEqual(self.identity(), self.identity("race"))

    def test_cached_result_reuse_failure_invalidation_and_concurrency(self):
        cache = self.root / ".cache"
        execute = Mock()
        checks.cached_lane(cache, "audio-unit", lambda: "old", execute)
        checks.cached_lane(cache, "audio-unit", lambda: "old", execute)
        self.assertEqual(execute.call_count, 1)
        execute.side_effect = ValueError("compiler failed")
        with self.assertRaisesRegex(ValueError, "compiler failed"):
            checks.cached_lane(cache, "audio-unit", lambda: "new", execute)
        self.assertFalse(checks.reusable(cache / "audio-unit.json", "old"))
        (cache / "audio-unit.running").mkdir()
        with self.assertRaisesRegex(ValueError, "already running"):
            checks.cached_lane(cache, "audio-unit", lambda: "new", execute)
        self.assertEqual(execute.call_count, 2)

    def test_changing_inputs_and_malformed_proof_cannot_pass(self):
        cache = self.root / ".cache"
        with self.assertRaisesRegex(ValueError, "changed during"):
            checks.cached_lane(cache, "audio-unit", Mock(side_effect=["before", "after"]), Mock())
        record = cache / "audio-unit.json"
        self.assertFalse(record.exists())
        record.write_text("malformed")
        execute = Mock()
        checks.cached_lane(cache, "audio-unit", lambda: "current", execute)
        execute.assert_called_once()

    def test_plan_does_not_create_results_or_run_work(self):
        cache = self.root / "absent-cache"
        execute = Mock()
        checks.cached_lane(cache, "audio-unit", lambda: "current", execute, plan=True)
        self.assertFalse(cache.exists())
        execute.assert_not_called()

    def test_failed_identity_probe_invalidates_old_success(self):
        cache = self.root / ".cache"
        checks.cached_lane(cache, "audio-unit", lambda: "old", Mock())
        with self.assertRaisesRegex(ValueError, "tool missing"):
            checks.cached_lane(cache, "audio-unit", Mock(side_effect=ValueError("tool missing")), Mock())
        self.assertFalse(checks.reusable(cache / "audio-unit.json", "old"))

    def test_skipped_or_incomplete_test_output_is_rejected(self):
        for action, package in (("skip", "example/audio"), ("pass", "example/other")):
            output = "\n".join(json.dumps(event) for event in (
                {"Action": "pass", "Package": package, "Test": "TestOwnership"},
                {"Action": action, "Package": package}))
            listed = json.dumps({"ImportPath": "example/audio", "TestGoFiles": ["audio_test.go"]})
            with self.subTest(action=action), patch.object(checks, "run", side_effect=[listed, output]), \
                    self.assertRaisesRegex(ValueError, "incomplete"):
                checks.test_packages(self.directory, {})

    def test_extra_modules_replacements_and_embeds_require_registration(self):
        manifest = {"schema_version": 2, "stage": "runtime", "modules": [self.module]}
        for name in ("LICENSE", "doc.go", "go.sum", "testdata/consumer/go.mod"):
            (self.directory / name).write_text("fixture\n")
        go_mod = self.directory / "go.mod"
        go_mod.write_text("module " + checks.INVENTORY.import_path(self.module) + "\n")
        paths = lambda: list(self.root.rglob("*"))
        checks.inventory(self.root, manifest, paths())
        extra = self.directory / "testdata/go.mod"
        extra.write_text("module hidden\n")
        with self.assertRaisesRegex(ValueError, "unregistered"):
            checks.inventory(self.root, manifest, paths())
        extra.unlink()
        go_mod.write_text(go_mod.read_text() + "replace other => ../other\n")
        with self.assertRaisesRegex(ValueError, "replacements"):
            checks.inventory(self.root, manifest, paths())
        go_mod.write_text("module " + checks.INVENTORY.import_path(self.module) + "\n")
        (self.directory / "runtime.go").write_text("package audio\n//go:embed README.md\n")
        with self.assertRaisesRegex(ValueError, "embedded asset"):
            checks.inventory(self.root, manifest, paths())

    def test_build_only_packages_are_distinct_from_skipped_or_missing_tests(self):
        expected, tested = {"audio", "audio/cmd/tool"}, {"audio"}
        events = [{"Package": "audio", "Test": "TestOwnership", "Action": "pass"},
                  {"Package": "audio", "Action": "pass"},
                  {"Package": "audio/cmd/tool", "Action": "skip"}]
        self.assertEqual(checks.verify_events(events, expected, tested), 1)
        for changed in (events[:-1], events + [events[-1]],
                        [dict(event, Action="skip") if event.get("Test") else event for event in events],
                        [event for event in events if not event.get("Test")]):
            with self.subTest(events=changed), self.assertRaisesRegex(ValueError, "incomplete"):
                checks.verify_events(changed, expected, tested)

    def test_candidate_dependencies_invalidate_only_candidate_mode(self):
        other = {"id": "modules/berad-text", "checks": list(checks.INVENTORY.BASE_LANES), "requires": []}
        directory = self.root / other["id"]
        directory.mkdir()
        path = directory / "layout.go"
        path.write_text("package text\n")
        identity = lambda: checks.fingerprint(self.root, self.module, "candidate", "policy", "tools", [other])
        before, published = identity(), self.identity()
        path.write_text("package text\n// changed candidate\n")
        self.assertNotEqual(before, identity())
        self.assertEqual(published, self.identity())

    def test_v2_tools_identity_and_focused_downstream_selection(self):
        runtime = self.module
        tools = {"id": "tools/berad-audio", "requires": [runtime["id"]],
                 "checks": [*checks.INVENTORY.BASE_LANES, "candidate"]}
        modules = checks.INVENTORY.normalize({"schema_version": 2, "stage": "runtime", "modules": [runtime, tools]})
        jobs = checks.INVENTORY.selected_jobs(modules, [runtime["id"]])
        self.assertEqual([(item["id"], lane) for item, lane, _ in jobs],
                         [(runtime["id"], lane) for lane in runtime["checks"]] + [(tools["id"], "candidate")])
        self.assertEqual([item["id"] for item in jobs[-1][2]], [runtime["id"]])
        jobs = checks.INVENTORY.selected_jobs(modules, [tools["id"]])
        self.assertEqual({item["id"] for item, _, _ in jobs}, {tools["id"]})
        with self.assertRaisesRegex(ValueError, "unknown"):
            checks.INVENTORY.selected_jobs(modules, ["tools/absent"])

    def test_manifest_rejects_escaping_duplicate_missing_and_cyclic_dependencies(self):
        import copy
        for issue in ("escape", "duplicate", "missing", "cycle", "tools-dependency"):
            runtime = copy.deepcopy(self.module)
            tools = {"id": "tools/berad-audio", "requires": [runtime["id"]],
                     "checks": [*checks.INVENTORY.BASE_LANES, "candidate"]}
            if issue == "escape":
                tools["id"] = "tools/../escape"
            elif issue == "duplicate":
                tools = copy.deepcopy(runtime)
            elif issue == "missing":
                tools["requires"] = ["modules/missing"]
            else:
                runtime["requires"] = [runtime["id"] if issue == "cycle" else tools["id"]]
                runtime["checks"].append("candidate")
            with self.subTest(issue=issue), self.assertRaises(ValueError):
                checks.INVENTORY.normalize({"schema_version": 2, "stage": "runtime", "modules": [runtime, tools]})

    def test_schema_one_remains_readable_with_namespaced_normalization(self):
        manifest = {"schema_version": 1, "stage": "runtime", "modules": [
            {"name": "berad-audio", "checks": list(checks.INVENTORY.BASE_LANES)}]}
        self.assertEqual(checks.INVENTORY.normalize(manifest), [self.module])

    def test_native_render_execution_is_registered_and_binds_backend_environment(self):
        module = dict(self.module, execution="native-render")
        manifest = {"schema_version": 2, "stage": "runtime", "modules": [module]}
        self.assertEqual(checks.INVENTORY.normalize(manifest), [module])
        identity = lambda: checks.fingerprint(self.root, module, "unit", "policy", "tools")
        with patch.dict(checks.os.environ, {"EBITENGINE_GRAPHICS_LIBRARY": "opengl"}):
            first = identity()
        with patch.dict(checks.os.environ, {"EBITENGINE_GRAPHICS_LIBRARY": "metal"}):
            self.assertNotEqual(first, identity())
        module["execution"] = "silently-skip-rendering"
        with self.assertRaises(ValueError):
            checks.INVENTORY.normalize(manifest)


if __name__ == "__main__":
    unittest.main()
