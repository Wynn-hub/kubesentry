#!/usr/bin/env python3
"""Small repository-local contract gate for the checked-in OpenAPI files."""

from pathlib import Path
import sys

import yaml


ROOT = Path(__file__).resolve().parents[1]


def fail(message: str) -> None:
    print(f"api contract error: {message}", file=sys.stderr)
    raise SystemExit(1)


def check_operations(document: dict, source: str, *, enforce_body_schemas: bool = True) -> int:
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
            if enforce_body_schemas:
                check_operation_bodies(operation, f"{source} {method.upper()} {path}")
    return operations


def is_schema_ref(schema: object) -> bool:
    return isinstance(schema, dict) and set(schema) == {"$ref"} and isinstance(schema["$ref"], str)


def check_operation_bodies(operation: dict, location: str) -> None:
    """Require contract-owned schemas at the HTTP operation boundary.

    Inline property schemas remain useful inside the component fragments, but
    request bodies and successful response bodies must name a reusable schema.
    That makes an accidental ``additionalProperties: true`` endpoint visible to
    both ogen and openapi-typescript rather than silently becoming ``any``.
    """
    request_body = operation.get("requestBody")
    if isinstance(request_body, dict):
        for media in request_body.get("content", {}).values():
            if isinstance(media, dict) and not is_schema_ref(media.get("schema")):
                fail(f"{location} request body must reference an explicit schema")

    for status, response in operation.get("responses", {}).items():
        if not str(status).startswith("2") or not isinstance(response, dict):
            continue
        for media in response.get("content", {}).values():
            if isinstance(media, dict) and not is_schema_ref(media.get("schema")):
                fail(f"{location} success response must reference an explicit schema")


def check_schema_properties(node: object, location: str) -> None:
    if isinstance(node, dict):
        properties = node.get("properties")
        if isinstance(properties, dict):
            for name, schema in properties.items():
                if not isinstance(schema, dict) or "$ref" in schema or "allOf" in schema:
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


def resolve_pointer(document: object, pointer: str, location: str) -> None:
    if not pointer.startswith("#/"):
        fail(f"{location} has an unsupported reference fragment {pointer!r}")
    target = document
    for raw_part in pointer[2:].split("/"):
        part = raw_part.replace("~1", "/").replace("~0", "~")
        if not isinstance(target, dict) or part not in target:
            fail(f"{location} has unresolved reference {pointer}")
        target = target[part]


def check_references(node: object, source_path: Path, documents: dict[Path, dict], location: str = "$") -> None:
    if isinstance(node, dict):
        ref = node.get("$ref")
        if isinstance(ref, str):
            file_part, separator, fragment = ref.partition("#")
            target_path = (source_path.parent / file_part).resolve() if file_part else source_path
            if target_path not in documents:
                if not target_path.is_file():
                    fail(f"{source_path} {location} has unresolved reference {ref}")
                documents[target_path] = load(target_path)
            if separator:
                resolve_pointer(documents[target_path], "#" + fragment, f"{source_path} {location}")
            elif file_part:
                fail(f"{source_path} {location} has reference without a fragment {ref}")
        for name, child in node.items():
            check_references(child, source_path, documents, f"{location}.{name}")
    elif isinstance(node, list):
        for index, child in enumerate(node):
            check_references(child, source_path, documents, f"{location}[{index}]")


def main() -> None:
    console_path = (ROOT / "api/console/console-api.yaml").resolve()
    gateway_path = (ROOT / "api/internal/gateway.yaml").resolve()
    console = load(console_path)
    gateway = load(gateway_path)
    if console.get("openapi") != "3.1.0" or gateway.get("openapi") != "3.1.0":
        fail("all HTTP contracts must use OpenAPI 3.1.0")
    if "bearerAuth" not in console.get("components", {}).get("securitySchemes", {}):
        fail("console contract must declare bearerAuth")
    console_count = check_operations(console, "console")
    gateway_count = check_operations(gateway, "gateway", enforce_body_schemas=False)
    documents = {console_path: console, gateway_path: gateway}
    checked: set[Path] = set()
    while unchecked := set(documents) - checked:
        source_path = unchecked.pop()
        check_references(documents[source_path], source_path, documents)
        checked.add(source_path)
    check_schema_properties(console, "console")
    for schema in (ROOT / "api/schemas").glob("*.yaml"):
        check_schema_properties(load(schema), schema.name)
    print(f"OpenAPI contract OK: {console_count} console operations, {gateway_count} gateway operations")


if __name__ == "__main__":
    main()
