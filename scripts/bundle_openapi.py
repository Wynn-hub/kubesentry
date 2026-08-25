#!/usr/bin/env python3
"""Bundle the Console OpenAPI fragments into the embedded JSON document."""

from __future__ import annotations

import argparse
from copy import deepcopy
import json
from pathlib import Path
import sys
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "api/console/console-api.yaml"
DEFAULT_OUTPUT = ROOT / "internal/console/openapi_bundle.json"


def load(path: Path, documents: dict[Path, dict[str, Any]]) -> dict[str, Any]:
    path = path.resolve()
    if path not in documents:
        parsed = yaml.safe_load(path.read_text())
        if not isinstance(parsed, dict):
            raise ValueError(f"{path} does not contain an object")
        documents[path] = parsed
    return documents[path]


def resolve_pointer(document: Any, pointer: str) -> Any:
    if not pointer.startswith("#/"):
        raise ValueError(f"unsupported reference fragment {pointer!r}")
    target = document
    for raw_segment in pointer[2:].split("/"):
        segment = raw_segment.replace("~1", "/").replace("~0", "~")
        if not isinstance(target, dict) or segment not in target:
            raise ValueError(f"unresolved reference {pointer}")
        target = target[segment]
    return target


def component_address(pointer: str) -> tuple[str, str] | None:
    parts = pointer.removeprefix("#/").split("/")
    if len(parts) == 3 and parts[0] == "components":
        return parts[1], parts[2].replace("~1", "/").replace("~0", "~")
    return None


def bundle() -> dict[str, Any]:
    documents: dict[Path, dict[str, Any]] = {}
    source = SOURCE.resolve()
    bundled = deepcopy(load(source, documents))
    output_components = bundled.setdefault("components", {})
    imported: set[tuple[Path, str, str]] = set()
    component_names: dict[tuple[Path, str, str], str] = {}
    component_origins: dict[tuple[str, str], Path] = {}

    def local_component_name(component_source: Path, kind: str, name: str) -> str:
        key = (component_source, kind, name)
        if key in component_names:
            return component_names[key]

        candidate = name
        existing_source = component_origins.get((kind, candidate))
        if existing_source is not None and existing_source != component_source:
            candidate = f"{component_source.stem}-{name}"
            suffix = 2
            while (kind, candidate) in component_origins:
                candidate = f"{component_source.stem}-{name}-{suffix}"
                suffix += 1
        component_names[key] = candidate
        component_origins[(kind, candidate)] = component_source
        return candidate

    def import_component(component_source: Path, kind: str, name: str) -> None:
        key = (component_source, kind, name)
        if key in imported:
            return
        source_components = load(component_source, documents).get("components", {})
        if name not in source_components.get(kind, {}):
            raise ValueError(f"{component_source} is missing #/components/{kind}/{name}")
        imported.add(key)
        output_bucket = output_components.setdefault(kind, {})
        local_name = local_component_name(component_source, kind, name)
        output_bucket[local_name] = None
        output_bucket[local_name] = visit(deepcopy(source_components[kind][name]), component_source)

    def rewrite_ref(ref: str, current_source: Path) -> tuple[str, Path, str]:
        file_part, separator, fragment = ref.partition("#")
        target_source = (current_source.parent / file_part).resolve() if file_part else current_source
        if not separator:
            raise ValueError(f"reference without fragment {ref!r}")
        pointer = "#" + fragment
        if not pointer.startswith("#/"):
            raise ValueError(f"unsupported reference fragment {ref!r}")
        return pointer, target_source, file_part

    def visit(value: Any, current_source: Path) -> Any:
        if isinstance(value, list):
            for index, item in enumerate(value):
                value[index] = visit(item, current_source)
            return value
        if not isinstance(value, dict):
            return value

        ref = value.get("$ref")
        if isinstance(ref, str):
            pointer, target_source, file_part = rewrite_ref(ref, current_source)
            address = component_address(pointer)
            if target_source != source or file_part:
                if address:
                    kind, name = address
                    import_component(target_source, kind, name)
                    local_name = local_component_name(target_source, kind, name)
                    value = {"$ref": f"#/components/{kind}/{local_name}"}
                else:
                    value = visit(deepcopy(resolve_pointer(load(target_source, documents), pointer)), target_source)
            elif current_source != source and address:
                kind, name = address
                import_component(current_source, kind, name)
                local_name = local_component_name(current_source, kind, name)
                value = {"$ref": f"#/components/{kind}/{local_name}"}

        for key in list(value):
            value[key] = visit(value[key], current_source)
        return value

    return visit(bundled, source)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    rendered = (json.dumps(bundle(), ensure_ascii=False, indent=2) + "\n").encode()
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
