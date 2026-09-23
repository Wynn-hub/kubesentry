#!/usr/bin/env python3
"""Reject stale Go import paths and cross-plane implementation dependencies."""

from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parents[1]
MODULE = "github.com/Wynn-hub/kubesentry"
LEGACY_IMPORTS = (
    f"{MODULE}/internal/api/v1alpha1",
    f"{MODULE}/internal/dbrepo",
    f"{MODULE}/internal/dbmodel",
    f"{MODULE}/internal/dbmigrate",
    f"{MODULE}/internal/admissionobs",
    f"{MODULE}/internal/admissionpolicy",
    f"{MODULE}/internal/platformobs",
)
HUBLINK_FORBIDDEN = (
    f"{MODULE}/internal/gateway",
    f"{MODULE}/internal/database/repository",
)
GO_SOURCE_DIRS = ("api", "cmd", "hack", "internal", "test")
SINGLE_IMPORT = re.compile(r'^\s*import\s+(?:[\w.]+\s+)?["`]([^"`]+)["`]')
BLOCK_IMPORT = re.compile(r'^\s*(?:[\w.]+\s+)?["`]([^"`]+)["`]')


def go_imports(source: str) -> list[str]:
    imports: list[str] = []
    in_block = False
    for line in source.splitlines():
        stripped = line.strip()
        if not in_block:
            match = SINGLE_IMPORT.match(line)
            if match:
                imports.append(match.group(1))
                continue
            if stripped == "import (":
                in_block = True
            continue
        if stripped == ")":
            in_block = False
            continue
        match = BLOCK_IMPORT.match(line)
        if match:
            imports.append(match.group(1))
    return imports


def matches(import_path: str, forbidden: str) -> bool:
    return import_path == forbidden or import_path.startswith(forbidden + "/")


def violations(root: Path) -> list[str]:
    problems: list[str] = []
    source_paths = (
        source_path
        for directory in GO_SOURCE_DIRS
        if (root / directory).is_dir()
        for source_path in (root / directory).rglob("*.go")
    )
    for source_path in sorted(source_paths):
        relative = source_path.relative_to(root).as_posix()
        imports = go_imports(source_path.read_text())
        for import_path in imports:
            if any(matches(import_path, old) for old in LEGACY_IMPORTS):
                problems.append(f"{relative}: legacy import {import_path}")
            if (
                relative.startswith("internal/operator/hublink/")
                and not relative.endswith("_test.go")
                and any(matches(import_path, forbidden) for forbidden in HUBLINK_FORBIDDEN)
            ):
                problems.append(f"{relative}: forbidden hublink production import {import_path}")
            if (
                relative.startswith("api/kubesentry/v1alpha1/")
                and import_path.startswith(f"{MODULE}/internal/")
            ):
                problems.append(f"{relative}: forbidden public API import {import_path}")
    return problems


def main() -> None:
    problems = violations(ROOT)
    if problems:
        print("architecture boundary errors:", file=sys.stderr)
        for problem in problems:
            print(f"- {problem}", file=sys.stderr)
        raise SystemExit(1)
    print("Architecture boundaries OK")


if __name__ == "__main__":
    main()
