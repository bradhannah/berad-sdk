#!/usr/bin/env python3
"""Bounded startup of the built text demo; not physical interaction acceptance."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]


def startup(command, cwd, environment, seconds=8):
    """Reject early exits, then terminate/reap this known single-process demo."""
    started = time.monotonic()
    with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as output:
        child = subprocess.Popen(command, cwd=cwd, env=environment,
                                 stdin=subprocess.DEVNULL, stdout=output,
                                 stderr=subprocess.STDOUT)
        try:
            try:
                code = child.wait(timeout=seconds)
            except subprocess.TimeoutExpired:
                pass
            else:
                output.seek(0)
                raise RuntimeError(f"demo exited before startup window: exit={code}; " + output.read(4096))
        finally:
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait(timeout=5)
        return {"startup": "survived", "required_seconds": seconds,
                "elapsed_seconds": round(time.monotonic() - started, 3),
                "controlled_stop_exit": child.returncode,
                "acceptance": "bounded native startup only; not keyboard/mouse/IME acceptance"}


def main():
    kind = sys.argv[1] if len(sys.argv) == 2 else "text"
    if kind not in ("text", "graphics") or len(sys.argv) > 2:
        raise ValueError("select text or graphics demo")
    name = "berad-" + kind + "-demo"
    executable = ROOT / ".cache/bin" / (name + ".exe" if os.name == "nt" else name)
    if not executable.is_file():
        raise RuntimeError("build the demo with make build-" + kind + "-demo first")
    with tempfile.TemporaryDirectory(prefix="text-demo-smoke-", dir=ROOT / ".cache") as directory:
        environment = dict(os.environ, HOME=directory, USERPROFILE=directory)
        result = startup([str(executable)], directory, environment)
        print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
