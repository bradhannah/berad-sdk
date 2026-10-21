"""A live process is required; a clean early exit is not startup acceptance."""

# Test names document the process lifecycle assertions.
# pylint: disable=missing-function-docstring

import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "text_demo_smoke", ROOT / "scripts/text_demo_smoke.py"
)
if SPEC is None or SPEC.loader is None:
    raise ImportError("cannot load SDK smoke probe under test")
smoke = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(smoke)


class TextDemoSmokeTests(unittest.TestCase):
    """Prove live process acceptance and bounded terminate/kill cleanup."""

    def test_early_success_or_failure_never_passes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            for code in (0, 7):
                with (
                    self.subTest(code=code),
                    self.assertRaisesRegex(RuntimeError, "exited before"),
                ):
                    smoke.startup(
                        [sys.executable, "-c", f"raise SystemExit({code})"],
                        directory,
                        dict(os.environ),
                        seconds=3,
                    )

    def test_running_process_is_stopped_and_reaped(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            result = smoke.startup(
                [sys.executable, "-c", "import time; time.sleep(10)"],
                directory,
                dict(os.environ),
                seconds=0.1,
            )
        self.assertEqual(result["startup"], "survived")
        self.assertIsNotNone(result["controlled_stop_exit"])

    def test_termination_timeout_escalates_and_reaps(self) -> None:
        child = Mock()
        child.wait.side_effect = [
            subprocess.TimeoutExpired("demo", 8),
            subprocess.TimeoutExpired("demo", 5),
            -9,
        ]
        child.poll.return_value = None
        child.returncode = -9
        with patch.object(smoke.subprocess, "Popen", return_value=child):
            result = smoke.startup(["demo"], ROOT, {})
        child.terminate.assert_called_once()
        child.kill.assert_called_once()
        self.assertEqual(child.wait.call_count, 3)
        self.assertEqual(result["controlled_stop_exit"], -9)


if __name__ == "__main__":
    unittest.main()
