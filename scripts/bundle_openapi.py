#!/usr/bin/env python3
"""Derive the embedded OpenAPI 3.0 document from generated Swagger 2."""

from __future__ import annotations

import argparse
from pathlib import Path
import sys
import tempfile

from convert_swagger_to_openapi import ConversionError, run_conversion
from normalize_openapi_json import normalize


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "api/swagger/swagger.yaml"
DEFAULT_OUTPUT = ROOT / "internal/console/openapi_bundle.json"


def render() -> bytes:
    with tempfile.TemporaryDirectory() as directory:
        converted = Path(directory) / "openapi.json"
        normalized = Path(directory) / "normalized.json"
        run_conversion(SOURCE, converted)
        normalize(converted, normalized)
        return normalized.read_bytes()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    try:
        rendered = render()
    except (ConversionError, OSError, ValueError) as exc:
        print(f"OpenAPI bundle generation failed: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc

    output = args.output.resolve()
    if args.check:
        if not output.is_file() or output.read_bytes() != rendered:
            print(f"OpenAPI bundle drift: run {Path(__file__).relative_to(ROOT)}", file=sys.stderr)
            raise SystemExit(1)
        return
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(rendered)


if __name__ == "__main__":
    main()
