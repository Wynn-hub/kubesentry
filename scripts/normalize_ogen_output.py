#!/usr/bin/env python3
"""Canonicalize ogen's non-semantic generated Go formatting deterministically."""

from pathlib import Path
import re
import sys


TYPE_SPEC = re.compile(r"^\t[A-Za-z_]")


def normalize_go_source(source: str) -> str:
    lines = source.splitlines()
    normalized: list[str] = []
    index = 0
    while index < len(lines):
        if lines[index] != "type (":
            normalized.append(lines[index])
            index += 1
            continue
        index += 1
        declaration: list[str] = []
        while index < len(lines) and lines[index] != ")":
            line = lines[index]
            if TYPE_SPEC.match(line):
                if declaration:
                    normalized.extend(declaration)
                declaration = ["type " + line[1:]]
            elif declaration:
                declaration.append(line[1:] if line.startswith("\t") else line)
            index += 1
        if declaration:
            normalized.extend(declaration)
        if index < len(lines):
            index += 1
    return "\n".join(line for line in normalized if line.strip()) + "\n"


def normalize_directory(output: Path) -> None:
    for path in output.glob("oas_*_gen.go"):
        path.write_text(normalize_go_source(path.read_text()))


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit(f"usage: {Path(sys.argv[0]).name} <ogen-output-directory>")
    normalize_directory(Path(sys.argv[1]))


if __name__ == "__main__":
    main()
