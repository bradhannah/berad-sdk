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
        self.module = {"name": "berad-audio", "checks": list(checks.LANES)}
        self.directory = self.root / "modules/berad-audio"
        (self.directory / "testdata/consumer").mkdir(parents=True)
        (self.directory / "runtime.go").write_text("package audio\n")
        (self.directory / "README.md").write_text("# Audio\n")
        (self.directory / "testdata/consumer/consumer_test.go").write_text("package consumer_test\n")

    def identity(self, lane="unit", policy="policy", tools="tools"):
        return checks.fingerprint(self.root, self.module, lane, policy, tools)

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
            with self.subTest(action=action), patch.object(checks, "run", side_effect=["example/audio\n", output]), \
                    self.assertRaisesRegex(ValueError, "incomplete"):
                checks.test_packages(self.directory, {})

    def test_extra_modules_replacements_and_embeds_require_registration(self):
        manifest = {"schema_version": 1, "stage": "runtime", "modules": [self.module]}
        for name in ("LICENSE", "doc.go", "go.sum", "testdata/consumer/go.mod"):
            (self.directory / name).write_text("fixture\n")
        go_mod = self.directory / "go.mod"
        go_mod.write_text("module " + checks.PREFIX + "berad-audio\n")
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
        go_mod.write_text("module " + checks.PREFIX + "berad-audio\n")
        (self.directory / "runtime.go").write_text("package audio\n//go:embed README.md\n")
        with self.assertRaisesRegex(ValueError, "embedded asset"):
            checks.inventory(self.root, manifest, paths())


if __name__ == "__main__":
    unittest.main()
