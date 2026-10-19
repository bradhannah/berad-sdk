"""Fault and invalidation regressions for the small foundation checker."""

import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import Mock

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("sdk_check", ROOT / "scripts/check.py")
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class ContractTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "sdk"
        def ignored(directory, names):
            relative = Path(directory).relative_to(ROOT)
            return [name for name in names if name in check.IGNORED or
                    check.opencode_output(relative / name) or
                    check.MODULE_CHECKS.INVENTORY.tool_output(relative / name)]
        shutil.copytree(ROOT, self.root, ignore=ignored)

    def test_foundation_is_self_contained(self):
        check.contracts(self.root)

    def test_generated_tool_bin_is_pruned_without_hiding_nested_source(self):
        generated = self.root / "tools/berad-audio/bin"
        generated.mkdir(parents=True, exist_ok=True)
        (generated / "audio-editor").write_bytes(b"executable")
        (generated / "not-source.go").write_text("not part of validation input\n")
        paths = check.files(self.root)
        self.assertFalse(any(path.is_relative_to(generated) for path in paths))
        nested = self.root / "tools/berad-audio/internal/bin"
        nested.mkdir(parents=True, exist_ok=True)
        (nested / "source.go").write_text("package bin\n")
        self.assertIn(nested / "source.go", check.files(self.root))
        check.contracts(self.root)

    def test_unregistered_go_code_is_rejected(self):
        (self.root / "modules/unregistered.go").write_text("package example\n")
        with self.assertRaisesRegex(ValueError, "unregistered"):
            check.contracts(self.root)

    def test_opencode_bootstrap_outputs_do_not_hide_skills_or_other_source(self):
        generated = self.root / ".opencode/node_modules/.bin"
        generated.mkdir(parents=True)
        (generated / "bootstrap").symlink_to(ROOT / "README.md")
        for name in ("package.json", "package-lock.json", "bun.lock", ".gitignore"):
            (self.root / ".opencode" / name).write_text("generated output\n")
        paths = check.files(self.root)
        self.assertFalse(any(check.opencode_output(path.relative_to(self.root)) for path in paths))
        self.assertIn(self.root / ".opencode/skill/berad-audio-development/SKILL.md", paths)
        check.contracts(self.root)
        authored = self.root / ".opencode/plugin"
        authored.mkdir()
        (authored / "borrowed.ts").symlink_to(ROOT / "README.md")
        with self.assertRaisesRegex(ValueError, "source symlink"):
            check.files(self.root)

    def test_manifest_cannot_claim_unsupported_module_coverage(self):
        (self.root / "sdk.json").write_text(json.dumps({"schema_version": 1, "stage": "active", "modules": ["audio"]}))
        with self.assertRaisesRegex(ValueError, "register module validation"):
            check.contracts(self.root)

    def test_broken_links_and_uncataloged_documents_fail(self):
        readme = self.root / "README.md"
        readme.write_text(readme.read_text() + "\n[missing](missing.md)\n")
        with self.assertRaisesRegex(ValueError, "broken or external"):
            check.contracts(self.root)
        readme.write_text((ROOT / "README.md").read_text())
        (self.root / "docs/unlisted.md").write_text("# Unlisted\n")
        with self.assertRaisesRegex(ValueError, "cataloged exactly once"):
            check.contracts(self.root)

    def test_external_source_symlink_is_rejected(self):
        (self.root / "borrowed.md").symlink_to(ROOT / "README.md")
        with self.assertRaisesRegex(ValueError, "source symlink"):
            check.contracts(self.root)

    def test_module_catalog_and_skill_disposition_are_checked(self):
        docs = self.root / "modules/berad-audio/docs"
        (docs / "unlisted.md").write_text("# Unlisted module contract\n")
        with self.assertRaisesRegex(ValueError, "cataloged exactly once"):
            check.contracts(self.root)
        (docs / "unlisted.md").unlink()
        skill = self.root / ".opencode/skill/berad-audio-development/SKILL.md"
        duplicate = self.root / ".opencode/skill/nested/berad-audio-development"
        duplicate.mkdir(parents=True)
        header = skill.read_text().split("---", 2)[1]
        (duplicate / "SKILL.md").write_text("---" + header + "---\n")
        with self.assertRaisesRegex(ValueError, "duplicate public SDK skill"):
            check.contracts(self.root)
        (duplicate / "SKILL.md").write_text("---\nname: wrong\ndescription: Use when testing.\n---\n")
        with self.assertRaisesRegex(ValueError, "invalid public SDK skill"):
            check.contracts(self.root)

    def test_real_fingerprint_ignores_prose_but_tracks_checker_changes(self):
        before = check.fingerprint(self.root)
        readme = self.root / "README.md"
        readme.write_text(readme.read_text() + "\nDocumentation-only clarification.\n")
        self.assertEqual(before, check.fingerprint(self.root))
        script = self.root / "scripts/check.py"
        script.write_text(script.read_text() + "\n# Changed validation policy.\n")
        self.assertNotEqual(before, check.fingerprint(self.root))


class ReuseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.cache = self.root / "cache"
        self.runner = Mock()

    def test_unchanged_inputs_reuse_and_changed_inputs_execute(self):
        for identity in ("first", "first", "changed"):
            check.regression_tests(self.root, self.cache, lambda _: identity, self.runner)
        self.assertEqual(self.runner.call_count, 2)

    def test_failure_supersedes_old_success(self):
        check.regression_tests(self.root, self.cache, lambda _: "old", self.runner)
        self.runner.side_effect = subprocess.CalledProcessError(1, "tests")
        with self.assertRaises(subprocess.CalledProcessError):
            check.regression_tests(self.root, self.cache, lambda _: "new", self.runner)
        self.assertFalse(check.reusable(self.cache / "checker-tests.json", "old"))
        self.assertFalse((self.cache / "running").exists())

    def test_mid_run_changes_do_not_create_success(self):
        identity = Mock(side_effect=["before", "after"])
        with self.assertRaisesRegex(ValueError, "changed during"):
            check.regression_tests(self.root, self.cache, identity, self.runner)
        self.assertFalse((self.cache / "checker-tests.json").exists())

    def test_concurrent_request_does_not_launch_duplicate(self):
        (self.cache / "running").mkdir(parents=True)
        with self.assertRaisesRegex(ValueError, "already running"):
            check.regression_tests(self.root, self.cache, lambda _: "same", self.runner)
        self.runner.assert_not_called()

    def test_malformed_record_reruns(self):
        self.cache.mkdir()
        (self.cache / "checker-tests.json").write_text("not json")
        check.regression_tests(self.root, self.cache, lambda _: "current", self.runner)
        self.runner.assert_called_once()


if __name__ == "__main__":
    unittest.main()
