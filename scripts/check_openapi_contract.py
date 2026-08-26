#!/usr/bin/env python3
"""Small repository-local contract gate for the checked-in OpenAPI files."""

from pathlib import Path
import re
import sys

import yaml


ROOT = Path(__file__).resolve().parents[1]
CONSOLE_OPERATION_COUNT = 50
GATEWAY_METHODS = frozenset({"Register", "Sync", "Connect", "NotifyChange", "Dispatch"})
CRD_NAMES = frozenset({
    "policies.kubesentry.io",
    "policygroups.kubesentry.io",
    "policyexceptions.kubesentry.io",
    "policyversions.kubesentry.io",
})
GENERATED_OPERATION = re.compile(r"// (?P<method>\w+) implements (?P<operation>\w+) operation\.")
HANDLER_METHOD = re.compile(r"func \(h \*Handler\) (?P<method>\w+)\(")


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
            if path.startswith(("/distributions", "/platform")):
                for parameter in operation.get("parameters", []):
                    if isinstance(parameter, dict) and "$ref" not in parameter:
                        check_parameter_metadata(parameter, f"{source} {method.upper()} {path} parameter {parameter.get('name', '<unnamed>')}")
            if enforce_body_schemas:
                check_operation_bodies(operation, f"{source} {method.upper()} {path}")
    return operations


def operation_ids(document: dict) -> set[str]:
    return {
        operation["operationId"]
        for item in document.get("paths", {}).values()
        if isinstance(item, dict)
        for method, operation in item.items()
        if method != "parameters" and isinstance(operation, dict) and isinstance(operation.get("operationId"), str)
    }


def check_console_route_inventory(operations: set[str]) -> None:
    if len(operations) != CONSOLE_OPERATION_COUNT:
        fail(f"expected {CONSOLE_OPERATION_COUNT} console operations, found {len(operations)}")


def check_generated_handler_coverage(operations: set[str], generated_path: Path, handler_dir: Path) -> None:
    try:
        generated = generated_path.read_text()
    except OSError as exc:
        fail(f"cannot read generated handler contract {generated_path}: {exc}")
    generated_operations = {match.group("operation"): match.group("method") for match in GENERATED_OPERATION.finditer(generated)}
    if set(generated_operations) != operations:
        missing = sorted(operations - set(generated_operations))
        extra = sorted(set(generated_operations) - operations)
        fail(f"generated Go operations drift from OpenAPI (missing={missing}, extra={extra})")
    methods = {
        match.group("method")
        for path in handler_dir.glob("*.go")
        if not path.name.endswith("_test.go")
        for match in HANDLER_METHOD.finditer(path.read_text())
    }
    missing = sorted(method for method in generated_operations.values() if method not in methods)
    if missing:
        fail(f"generated operations have no concrete handler method: {', '.join(missing)}")


def check_api_index(document: dict) -> None:
    boundaries = document.get("x-contract-boundaries")
    if not isinstance(boundaries, dict):
        fail("API index is missing x-contract-boundaries")
    console = boundaries.get("console")
    if not isinstance(console, dict) or console.get("source") != "./console/console-api.yaml" or console.get("routeCount") != CONSOLE_OPERATION_COUNT:
        fail("console contract must be OpenAPI-first with the complete route inventory")
    gateway = boundaries.get("gateway")
    if not isinstance(gateway, dict) or gateway.get("kind") != "protobuf" or gateway.get("source") != "./proto/gateway/v1/gateway.proto":
        fail("gateway contract must be protobuf-first")
    crds = boundaries.get("memberClusterCRDs")
    if not isinstance(crds, dict) or crds.get("kind") != "kubernetes-crd" or len(crds.get("sources", [])) != len(CRD_NAMES):
        fail("member-cluster CRD contract index is incomplete")


def check_gateway_proto() -> None:
    source = ROOT / "api/proto/gateway/v1/gateway.proto"
    try:
        methods = set(re.findall(r"^\s*rpc\s+(\w+)\(", source.read_text(), re.MULTILINE))
    except OSError as exc:
        fail(f"cannot read gateway protobuf contract: {exc}")
    if methods != GATEWAY_METHODS:
        fail(f"gateway protobuf methods differ from the indexed contract (found={sorted(methods)})")


def check_crd_contracts() -> None:
    names: set[str] = set()
    for path in (ROOT / "charts/kubesentry/crds").glob("*.yaml"):
        document = load(path)
        if document.get("kind") == "CustomResourceDefinition":
            name = document.get("metadata", {}).get("name")
            if isinstance(name, str):
                names.add(name)
    if names != CRD_NAMES:
        fail(f"member-cluster CRD inventory differs from the indexed contract (found={sorted(names)})")


def check_parameter_metadata(parameter: dict, location: str) -> None:
    if not parameter.get("description"):
        fail(f"{location} is missing description")
    schema = parameter.get("schema")
    if not isinstance(schema, dict) or "$ref" in schema:
        return
    if not schema.get("description"):
        fail(f"{location} parameter schema is missing description")
    if schema.get("type") not in ("object", "array") and "example" not in schema:
        fail(f"{location} parameter schema is missing example")


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


def check_free_form_objects(node: object, location: str) -> None:
    """Allow raw JSON only for admission resource snapshots.

    Contracts for all other objects must expose their fields or, for typed maps,
    declare the value schema through ``additionalProperties: { ... }``.
    """
    if isinstance(node, dict):
        if node.get("additionalProperties") is True and not (
            location.startswith("admission.yaml.")
            and (location.endswith(".metadataSnapshot") or location.endswith(".specSnapshot"))
        ):
            fail(f"{location} has unapproved additionalProperties: true")
        for name, child in node.items():
            check_free_form_objects(child, f"{location}.{name}")
    elif isinstance(node, list):
        for index, child in enumerate(node):
            check_free_form_objects(child, f"{location}[{index}]")


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
    index_path = (ROOT / "api/openapi.yaml").resolve()
    console = load(console_path)
    index = load(index_path)
    if console.get("openapi") != "3.1.0" or index.get("openapi") != "3.1.0":
        fail("Console contract and API index must use OpenAPI 3.1.0")
    if "bearerAuth" not in console.get("components", {}).get("securitySchemes", {}):
        fail("console contract must declare bearerAuth")
    console_count = check_operations(console, "console")
    operations = operation_ids(console)
    check_console_route_inventory(operations)
    check_generated_handler_coverage(operations, ROOT / "internal/console/dto/oas_server_gen.go", ROOT / "internal/console/handler")
    check_api_index(index)
    check_gateway_proto()
    check_crd_contracts()
    documents = {console_path: console, index_path: index}
    checked: set[Path] = set()
    while unchecked := set(documents) - checked:
        source_path = unchecked.pop()
        check_references(documents[source_path], source_path, documents)
        checked.add(source_path)
    check_schema_properties(console, "console")
    for schema in (ROOT / "api/schemas").glob("*.yaml"):
        document = load(schema)
        check_schema_properties(document, schema.name)
        check_free_form_objects(document, schema.name)
    print(f"API contract OK: {console_count} console operations, {len(GATEWAY_METHODS)} gateway RPCs, {len(CRD_NAMES)} member-cluster CRDs")


if __name__ == "__main__":
    main()
