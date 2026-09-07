#!/usr/bin/env python3
"""Strictly convert generated Swagger 2 JSON/YAML to OpenAPI 3.0.0 JSON."""

import argparse
from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
CONVERTER = ROOT / "web/node_modules/.bin/swagger2openapi"
WARNING = re.compile(r"\bwarn(?:ing)?s?\b", re.IGNORECASE)


class ConversionError(RuntimeError):
    pass


def run_conversion(source: Path, output: Path, executable: Path = CONVERTER) -> None:
    result = subprocess.run(
        [str(executable), "--targetVersion", "3.0.0", "--outfile", str(output), str(source)],
        capture_output=True,
        text=True,
    )
    diagnostics = "\n".join(part for part in (result.stdout, result.stderr) if part)
    if result.returncode != 0 or WARNING.search(diagnostics):
        output.unlink(missing_ok=True)
        reason = "converter warning" if WARNING.search(diagnostics) else f"converter exited {result.returncode}"
        raise ConversionError(f"{reason}: {diagnostics.strip()}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    try:
        run_conversion(args.source, args.output)
    except (ConversionError, OSError) as exc:
        print(f"Swagger conversion failed: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc


if __name__ == "__main__":
    main()
