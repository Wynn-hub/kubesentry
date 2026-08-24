#!/usr/bin/env python3
"""Small repository-local contract gate for the checked-in OpenAPI files."""

from pathlib import Path
import sys

import yaml


ROOT = Path(__file__).resolve().parents[1]


def fail(message: str) -> None:
    print(f"api contract error: {message}", file=sys.stderr)
    raise SystemExit(1)


def check_operations(document: dict, source: str) -> int:
    operations = 0
    for path, item in document.get("paths", {}).items():
        if not isinstance(item, dict):
            continue
        for method, operation in item.items():
            if method == "parameters":
                continue
            operations += 1
            if not isinstance(operation, dict):
                fail(f"{source} {method.upper()} {path} is not an object")
            for key in ("tags", "operationId", "summary", "description", "responses"):
                if not operation.get(key):
                    fail(f"{source} {method.upper()} {path} is missing {key}")
    return operations


def check_schema_properties(node: object, location: str) -> None:
    if isinstance(node, dict):
        properties = node.get("properties")
        if isinstance(properties, dict):
            for name, schema in properties.items():
                if not isinstance(schema, dict) or "$ref" in schema:
                    continue
                if not schema.get("description"):
                    fail(f"{location}.{name} is missing description")
                if schema.get("type") not in ("object", "array") and "example" not in schema:
                    fail(f"{location}.{name} is missing example")
        for name, child in node.items():
            check_schema_properties(child, f"{location}.{name}")
    elif isinstance(node, list):
        for index, child in enumerate(node):
            check_schema_properties(child, f"{location}[{index}]")


def load(path: Path) -> dict:
    try:
        value = yaml.safe_load(path.read_text())
    except OSError as exc:
        fail(f"cannot read {path}: {exc}")
    except yaml.YAMLError as exc:
        fail(f"cannot parse {path}: {exc}")
    if not isinstance(value, dict):
        fail(f"{path} does not contain an object")
    return value


def main() -> None:
    console = load(ROOT / "api/console/console-api.yaml")
    gateway = load(ROOT / "api/internal/gateway.yaml")
    if console.get("openapi") != "3.1.0" or gateway.get("openapi") != "3.1.0":
        fail("all HTTP contracts must use OpenAPI 3.1.0")
    if "bearerAuth" not in console.get("components", {}).get("securitySchemes", {}):
        fail("console contract must declare bearerAuth")
    console_count = check_operations(console, "console")
    gateway_count = check_operations(gateway, "gateway")
    check_schema_properties(console, "console")
    for schema in (ROOT / "api/schemas").glob("*.yaml"):
        check_schema_properties(load(schema), schema.name)
    print(f"OpenAPI contract OK: {console_count} console operations, {gateway_count} gateway operations")


if __name__ == "__main__":
    main()
