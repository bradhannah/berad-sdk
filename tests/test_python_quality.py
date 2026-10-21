"""Real Python tool failures, candidate scope and content-bound reuse regressions."""

from __future__ import annotations

# Descriptive test names specify the contract without repeating assertion prose.
# pylint: disable=missing-function-docstring

import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import Mock, patch

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "sdk_python_quality", ROOT / "scripts/python_quality.py"
)
if SPEC is None or SPEC.loader is None:
    raise ImportError("cannot load Python quality helper under test")
quality = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(quality)


class PythonQualityTests(unittest.TestCase):
    """Small independent fixtures exercise tool policy and reuse failure boundaries."""

    def setUp(self) -> None:
        # unittest owns and closes this context across setup and test execution.
        # pylint: disable-next=consider-using-with
        self.root = Path(self.enterContext(tempfile.TemporaryDirectory()))
        for name in quality.POLICY:
            (self.root / name).write_bytes((ROOT / name).read_bytes())
        (self.root / ".githooks").mkdir()
        (self.root / ".githooks/pre-commit").write_bytes(
            (ROOT / ".githooks/pre-commit").read_bytes()
        )
        self.source = self.root / "fixture.py"
        self.source.write_text(
            '"""Independent tool fixture."""\nVALUE = 1\n', encoding="utf-8"
        )

    def test_real_black_failure_does_not_edit_then_format_repairs(self) -> None:
        self.source.write_text(
            '"""Independent fixture."""\nVALUE=  1\n', encoding="utf-8"
        )
        before = self.source.read_bytes()
        with patch.object(
            quality, "interpreter", return_value=quality.interpreter(ROOT)
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                quality.run_tool(self.root, "black", [self.source])
            self.assertEqual(before, self.source.read_bytes())
            quality.run_tool(self.root, "black", [self.source], write=True)
            quality.run_tool(self.root, "black", [self.source])

    def test_real_pylint_rejects_undefined_name_and_accepts_control(self) -> None:
        self.source.write_text(
            '"""Independent fixture."""\nVALUE = missing_name\n', encoding="utf-8"
        )
        with patch.object(
            quality, "interpreter", return_value=quality.interpreter(ROOT)
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                quality.run_tool(self.root, "pylint", [self.source])
            self.source.write_text(
                '"""Independent fixture."""\nVALUE = 1\n', encoding="utf-8"
            )
            quality.run_tool(self.root, "pylint", [self.source])

    def test_enabled_informational_diagnostics_also_fail(self) -> None:
        self.source.write_text(
            '"""Independent fixture."""\n'
            "# pylint: disable=undefined-variable\nVALUE = 1\n",
            encoding="utf-8",
        )
        with patch.object(
            quality, "interpreter", return_value=quality.interpreter(ROOT)
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                quality.run_tool(self.root, "pylint", [self.source])

    def test_inventory_includes_new_tests_ignores_cache_and_rejects_symlinks(
        self,
    ) -> None:
        new_test = self.root / "test_added.py"
        new_test.write_text('"""New authored test."""\n', encoding="utf-8")
        # Git supplies tracked plus nonignored new paths; caches are also pruned
        # defensively if a contributor previously tracked one.
        names = b"fixture.py\0test_added.py\0.cache/tool.py\0nested/.cache/tool.py\0"
        with patch.object(quality, "git", return_value=names):
            self.assertEqual(quality.source_files(self.root), [self.source, new_test])
        linked = self.root / "linked"
        linked.symlink_to(ROOT / "scripts", target_is_directory=True)
        with patch.object(quality, "git", return_value=b"fixture.py\0linked\0"):
            with self.assertRaisesRegex(ValueError, "symlink"):
                quality.source_files(self.root)

    def test_real_pylint_checks_hidden_filenames(self) -> None:
        hidden = self.root / ".hidden.py"
        hidden.write_text(
            '"""Hidden source."""\nVALUE = missing_name\n', encoding="utf-8"
        )
        with patch.object(
            quality, "interpreter", return_value=quality.interpreter(ROOT)
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                quality.run_tool(self.root, "pylint", [hidden])

    def test_real_pylint_config_diagnostic_is_not_a_success(self) -> None:
        config = self.root / "pyproject.toml"
        config.write_text(
            config.read_text(encoding="utf-8")
            + "\n[tool.pylint.invalid]\nnot-a-pylint-option = true\n",
            encoding="utf-8",
        )
        with patch.object(
            quality, "interpreter", return_value=quality.interpreter(ROOT)
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                quality.run_tool(self.root, "pylint", [self.source])

    def test_pylint_payload_and_status_both_must_pass(self) -> None:
        with patch.object(quality, "interpreter", return_value=Path("python")):
            for output, status in (
                ("", 0),
                ("{}", 0),
                ('[{"message-id":"E0015"}]', 0),
                ("[]", 1),
            ):
                with (
                    self.subTest(output=output, status=status),
                    patch.object(
                        quality.subprocess,
                        "run",
                        return_value=subprocess.CompletedProcess(
                            [], status, output, ""
                        ),
                    ),
                ):
                    with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                        quality.run_tool(self.root, "pylint", [self.source])

    def test_provision_refuses_existing_or_symlinked_environments(self) -> None:
        cache = self.root / ".cache"
        cache.mkdir()
        environment = cache / "python-quality"
        environment.mkdir()
        sentinel = environment / "keep"
        sentinel.write_text("existing environment", encoding="utf-8")
        with patch.object(quality.venv, "EnvBuilder") as builder:
            with self.assertRaisesRegex(ValueError, "already exists"):
                quality.provision(self.root)
            self.assertEqual(
                sentinel.read_text(encoding="utf-8"), "existing environment"
            )
            sentinel.unlink()
            environment.rmdir()
            environment.symlink_to(self.root, target_is_directory=True)
            with self.assertRaisesRegex(ValueError, "symlinked"):
                quality.provision(self.root)
            with self.assertRaisesRegex(ValueError, "symlinked"):
                quality.interpreter(self.root)
            environment.unlink()
            cache.rmdir()
            cache.symlink_to(self.root, target_is_directory=True)
            with self.assertRaisesRegex(ValueError, "symlinked"):
                quality.provision(self.root)
            builder.assert_not_called()

    def test_pre_identity_failure_invalidates_only_requested_python_lanes(self) -> None:
        cache = self.root / ".cache/validation"
        cache.mkdir(parents=True)
        records = [
            cache / f"{name}.json" for name in ("python-format-check", "python-lint")
        ]
        for record in records:
            record.write_text("previous success", encoding="utf-8")
        with patch.object(
            quality, "load_module", side_effect=ValueError("loader failed")
        ):
            with self.assertRaisesRegex(ValueError, "loader failed"):
                quality.validate(self.root, plan=True)
            self.assertTrue(all(record.exists() for record in records))
            with self.assertRaisesRegex(ValueError, "loader failed"):
                quality.validate(self.root, "python-lint")
            self.assertTrue(records[0].exists())
            self.assertFalse(records[1].exists())

    def test_source_policy_lock_and_tools_invalidate_cached_checks(self) -> None:
        with (
            patch.object(quality, "git", return_value=b"fixture.py\0"),
            patch.object(quality, "tool_identity", return_value="tools") as tools,
            patch.object(quality, "run_tool") as run,
        ):
            quality.validate(self.root)
            quality.validate(self.root)
            self.assertEqual(run.call_count, 2)
            for name in ("fixture.py", "pyproject.toml", "requirements-quality.lock"):
                path = self.root / name
                path.write_bytes(path.read_bytes() + b"\n# changed input\n")
                quality.validate(self.root)
            self.assertEqual(run.call_count, 8)
            tools.return_value = "changed implementation"
            quality.validate(self.root)
            self.assertEqual(run.call_count, 10)

    def test_failed_probe_clears_old_success_and_plan_is_read_only(self) -> None:
        with (
            patch.object(quality, "git", return_value=b"fixture.py\0"),
            patch.object(quality, "tool_identity", return_value="tools") as tools,
            patch.object(quality, "run_tool") as run,
        ):
            quality.validate(self.root, plan=True)
            self.assertFalse((self.root / ".cache").exists())
            run.assert_not_called()
            quality.validate(self.root)
            tools.side_effect = ValueError("missing tool")
            with self.assertRaisesRegex(ValueError, "missing tool"):
                quality.validate(self.root)
            self.assertFalse(
                (self.root / ".cache/validation/python-format-check.json").exists()
            )

    def test_actual_dependency_bytes_change_identity_without_version_change(
        self,
    ) -> None:
        (self.root / "requirements-quality.lock").write_text(
            "astroid==4.0.4\n", encoding="utf-8"
        )
        dependency = self.root / "dependency.py"
        dependency.write_text("VALUE = 1\n", encoding="utf-8")
        distribution = Mock(version="4.0.4", files=[Path("dependency.py")])
        distribution.locate_file.side_effect = lambda relative: self.root / relative
        with (
            patch.object(quality, "runtime_identity", return_value="runtime"),
            patch.object(quality.sysconfig, "get_path", return_value=str(self.root)),
            patch.object(
                quality.importlib.metadata, "distribution", return_value=distribution
            ),
        ):
            before = quality.installed_identity(self.root)
            dependency.write_text("VALUE = 2\n", encoding="utf-8")
            self.assertNotEqual(before, quality.installed_identity(self.root))
            before = quality.installed_identity(self.root)
            # Added importable files outside RECORD still affect implementation.
            (self.root / "added_dependency.py").write_text(
                "VALUE = 3\n", encoding="utf-8"
            )
            self.assertNotEqual(before, quality.installed_identity(self.root))
            distribution.version = "4.0.3"
            with self.assertRaisesRegex(ValueError, "must be 4.0.4"):
                quality.installed_identity(self.root)

    def test_hook_rejects_unstaged_or_new_inputs_and_mid_check_index_changes(
        self,
    ) -> None:
        for responses in ([b"changed.py\0"], [b"", b"new.py\0"]):
            with (
                patch.object(quality, "git", side_effect=responses),
                patch.object(quality, "validate") as validate,
            ):
                with self.assertRaisesRegex(ValueError, "index matching"):
                    quality.pre_commit(self.root)
                validate.assert_not_called()
        with (
            patch.object(
                quality,
                "git",
                side_effect=[b"", b"", b"index-before", b"", b"", b"index-after"],
            ),
            patch.object(quality, "validate"),
        ):
            with self.assertRaisesRegex(ValueError, "candidate changed"):
                quality.pre_commit(self.root)
        with (
            patch.object(quality, "git", side_effect=[b"", b"", b"index"] * 2),
            patch.object(quality, "validate") as validate,
        ):
            quality.pre_commit(self.root)
            validate.assert_called_once_with(self.root)


if __name__ == "__main__":
    unittest.main()
