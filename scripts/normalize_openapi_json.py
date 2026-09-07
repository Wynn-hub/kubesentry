#!/usr/bin/env python3
"""Normalize derived OpenAPI JSON without changing semantic content."""

import argparse
import json
from pathlib import Path


def normalize(source: Path, output: Path) -> None:
    document = json.loads(source.read_text())
    output.write_text(json.dumps(document, indent=4, sort_keys=True, ensure_ascii=False) + "\n")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    normalize(args.source, args.output)


if __name__ == "__main__":
    main()
