"""Prove that formatting/lint are real failures and cached proof covers exact inputs."""

import importlib.util
import contextlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("sdk_style_test_check", ROOT / "scripts/check.py")
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)
style, checks = checker.STYLE_CHECKS, checker.MODULE_CHECKS


class StyleChecksTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.module = {"id": "modules/berad-text", "checks": ["unit", "race", "consumer"], "requires": []}
        self.directory = self.root / self.module["id"]
        self.directory.mkdir(parents=True)
        (self.directory / "source.go").write_text("package text\n")
        (self.root / ".golangci.yml").write_text("policy")

    def test_format_inventory_includes_nested_consumers_tests_and_internal_bin(self):
        for name in ("source_test.go", "testdata/consumer/consumer_test.go", "internal/bin/source.go"):
            path = self.directory / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("package text\n")
        self.assertEqual(len(style.source_files(self.root, [self.module], checks)), 4)
        (self.directory / "go.mod").write_text("module example.com/style\n\ngo 1.27.0\n")
        source = self.directory / "source.go"
        source.write_text("package text\nfunc answer( ) int{return 42}\n")
        env = checks.environment()
        with patch.object(style, "executable", side_effect=lambda _, name: style_tool(name)):
            with self.assertRaisesRegex(ValueError, "require make format"):
                style.formatting(self.root, [self.module], checks, env)
            self.assertIn("answer( )", source.read_text(), "checks must not silently format source")
            style.formatting(self.root, [self.module], checks, env, write=True)
            style.formatting(self.root, [self.module], checks, env)

    def test_real_linter_reports_ignored_error_and_unchecked_assertion(self):
        (self.root / ".cache").mkdir()
        (self.root / ".golangci.yml").write_bytes((ROOT / ".golangci.yml").read_bytes())
        (self.directory / "go.mod").write_text("module example.com/style\n\ngo 1.27.0\n")
        (self.directory / "source.go").write_text(
            'package text\nimport "os"\nfunc probe(v any) { _, _ = os.Open("x"); _ = v.(string) }\n')
        with patch.object(style, "executable", side_effect=lambda _, name: style_tool(name)):
            with self.assertRaises(subprocess.CalledProcessError), contextlib.redirect_stdout(io.StringIO()):
                style.lint_module(self.root, self.module, checks, checks.environment())
        report = json.loads((self.root / ".cache/modules-berad-text-lint.json").read_text())
        self.assertGreaterEqual(sum(item["FromLinter"] == "errcheck" for item in report["Issues"]), 2)

    def test_pinned_tool_identity_rejects_missing_or_wrong_build(self):
        with self.assertRaisesRegex(ValueError, "make tools"):
            style.tool_identity(self.root, checks, {})
        for name in style.PINS:
            path = style.executable(self.root, name)
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b"wrong-tool")
        with patch.object(checks, "run", return_value="mod wrong/tool v0.0.1"):
            with self.assertRaisesRegex(ValueError, "must be built from"):
                style.tool_identity(self.root, checks, {})

    def test_module_consumer_policy_and_tool_changes_invalidate_style_proof(self):
        with patch.object(checks, "tools", return_value="go"), \
                patch.object(style, "tool_identity", return_value="tools") as tools, \
                patch.object(style, "verify_config"), patch.object(style, "formatting") as formatting, \
                patch.object(style, "lint_module") as lint:
            execute = lambda: style.validate(self.root, [self.module], checks, lambda: "policy")
            execute()
            execute()
            self.assertEqual(lint.call_count, 1)
            self.assertEqual(formatting.call_count, 1)
            (self.directory / "testdata/consumer").mkdir(parents=True)
            (self.directory / "testdata/consumer/test.go").write_text("package consumer\n")
            execute()
            self.assertEqual(lint.call_count, 2)
            (self.root / ".golangci.yml").write_text("new policy")
            execute()
            self.assertEqual(lint.call_count, 3)
            tools.return_value = "new binary"
            execute()
            self.assertEqual(lint.call_count, 4)

    def test_format_failure_blocks_lint_and_failed_lint_cannot_reveal_old_pass(self):
        with patch.object(checks, "tools", return_value="go"), \
                patch.object(style, "tool_identity", return_value="tools"), \
                patch.object(style, "verify_config"), patch.object(style, "formatting") as formatting, \
                patch.object(style, "lint_module") as lint:
            execute = lambda: style.validate(self.root, [self.module], checks, lambda: "policy")
            formatting.side_effect = ValueError("unformatted")
            with self.assertRaisesRegex(ValueError, "unformatted"):
                execute()
            lint.assert_not_called()
            formatting.side_effect = None
            execute()
            (self.directory / "source.go").write_text("package text\n// changed\n")
            lint.side_effect = ValueError("lint findings")
            with self.assertRaisesRegex(ValueError, "lint findings"):
                execute()
            self.assertFalse((self.root / ".cache/validation/style/modules-berad-text-lint.json").exists())

    def test_full_inventory_and_module_selector_reach_each_production_module(self):
        modules = [self.module, dict(self.module, id="tools/berad-audio"),
                   dict(self.module, id="modules/berad-audio")]
        for module in modules[1:]:
            directory = self.root / module["id"]
            directory.mkdir(parents=True)
            (directory / "source.go").write_text("package audio\n")
        with patch.object(checks, "tools", return_value="go"), \
                patch.object(style, "tool_identity", return_value="tools"), \
                patch.object(style, "verify_config"), patch.object(style, "formatting"), \
                patch.object(style, "lint_module") as lint:
            style.validate(self.root, modules, checks, lambda: "policy", selectors=["tools/berad-audio"])
            self.assertEqual([call.args[1]["id"] for call in lint.call_args_list], ["tools/berad-audio"])
            style.validate(self.root, modules, checks, lambda: "policy")
            self.assertEqual({call.args[1]["id"] for call in lint.call_args_list}, {m["id"] for m in modules})

    def test_autofix_visits_all_selected_modules_but_never_reports_remaining_findings_as_pass(self):
        modules = [self.module, dict(self.module, id="modules/berad-audio")]
        with patch.object(checks, "tools", return_value="go"), \
                patch.object(style, "tool_identity", return_value="tools"), \
                patch.object(style, "verify_config"), \
                patch.object(style, "lint_module", side_effect=[subprocess.CalledProcessError(1, "lint"), None]) as lint:
            with self.assertRaisesRegex(ValueError, "automatic fixes left findings"):
                style.validate(self.root, modules, checks, lambda: "policy", operation="lint-fix")
            self.assertEqual([call.args[1]["id"] for call in lint.call_args_list], [m["id"] for m in modules])


def style_tool(name):
    return ROOT / ".cache/tools" / (name + (".exe" if style.os.name == "nt" else ""))


if __name__ == "__main__":
    unittest.main()
