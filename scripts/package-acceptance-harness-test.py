"""Black-box strict external-binary selection regression; pass compiled e2e test exe."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

TEST_EXE = str(Path(sys.argv.pop(1)).resolve())

class ExternalBinarySelection(unittest.TestCase):
    def test_invalid_explicit_values_never_fall_back_to_build(self):
        with tempfile.TemporaryDirectory(prefix="package-harness-", dir=os.environ["TMPDIR"]) as directory:
            invalid = Path(directory) / "not-an-executable.exe"
            invalid.write_text("not a program", encoding="utf-8")
            for value in ("", str(Path(directory) / "missing.exe"), directory, str(invalid)):
                with self.subTest(value=value):
                    env = dict(os.environ, UNAGIKUMA_E2E_BIN=value)
                    result = subprocess.run([TEST_EXE, "-test.run=^$"], env=env, cwd=Path(__file__).resolve().parents[1] / "e2e", capture_output=True, text=True, timeout=180)
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertIn("UNAGIKUMA_E2E_BIN:", result.stdout + result.stderr)
                    self.assertNotIn("building source binary", result.stdout + result.stderr)

if __name__ == "__main__":
    unittest.main()
