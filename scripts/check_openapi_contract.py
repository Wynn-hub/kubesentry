#!/usr/bin/env python3
"""Small repository-local contract gate for the checked-in OpenAPI files."""

from pathlib import Path
import hashlib
import json
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
HANDLER_OPERATION = re.compile(
    r"(?ms)^// @ID (?P<operation>\w+).*?^func \(h \*Handler\) (?P<method>\w+)\("
)
ROUTE_REGISTRATIONS = frozenset({
    "registerPolicyRoutes",
    "registerPolicyGroupRoutes",
    "registerExceptionRoutes",
    "registerSchemaAndSummaryRoutes",
    "registerAdmissionRoutes",
    "registerClusterRoutes",
    "registerDistributionRoutes",
    "registerPlatformRoutes",
})
RUNTIME_ROUTE = re.compile(r'mux\.HandleFunc\("(?P<method>[A-Z]+) (?P<path>/api/v1[^" ]*)"')


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


def schema_refs(node: object) -> list[str]:
    refs: set[str] = set()
    if isinstance(node, dict):
        ref = node.get("$ref")
        if isinstance(ref, str):
            refs.add(ref)
        for child in node.values():
            refs.update(schema_refs(child))
    elif isinstance(node, list):
        for child in node:
            refs.update(schema_refs(child))
    return sorted(refs)


def build_operation_inventory(document: dict) -> list[dict]:
    inventory: list[dict] = []
    for path, item in document.get("paths", {}).items():
        if not isinstance(item, dict):
            continue
        for method, operation in item.items():
            if method == "parameters" or not isinstance(operation, dict):
                continue
            parameters = []
            for parameter in operation.get("parameters", []):
                if not isinstance(parameter, dict):
                    continue
                if isinstance(parameter.get("$ref"), str):
                    parameters.append({"$ref": parameter["$ref"]})
                    continue
                parameters.append({
                    "in": parameter.get("in"),
                    "name": parameter.get("name"),
                    "required": parameter.get("required", False),
                    "schema": parameter.get("schema", {}),
                })
            responses = {
                str(status): schema_refs(response)
                for status, response in operation.get("responses", {}).items()
            }
            entry = {
                "method": method.upper(),
                "operationId": operation.get("operationId"),
                "parameters": parameters,
                "path": path,
                "responses": responses,
            }
            request_refs = schema_refs(operation.get("requestBody"))
            if request_refs:
                entry["requestBody"] = request_refs
            inventory.append(entry)
    return sorted(inventory, key=lambda entry: (entry["path"], entry["method"]))


def operation_inventory_snapshot(document: dict) -> dict:
    inventory = build_operation_inventory(document)
    encoded = json.dumps(inventory, sort_keys=True, separators=(",", ":")).encode()
    return {
        "operationCount": len(inventory),
        "operations": inventory,
        "sha256": hashlib.sha256(encoded).hexdigest(),
    }


def check_committed_inventory(document: dict, inventory_path: Path) -> None:
    try:
        committed = json.loads(inventory_path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read operation inventory {inventory_path}: {exc}")
    if committed != operation_inventory_snapshot(document):
        fail("operation inventory drift: regenerate the reviewed Console contract baseline")


def check_console_route_inventory(operations: set[str]) -> None:
    if len(operations) != CONSOLE_OPERATION_COUNT:
        fail(f"expected {CONSOLE_OPERATION_COUNT} console operations, found {len(operations)}")


def check_handler_coverage(operations: set[str], handler_dir: Path) -> None:
    annotated: dict[str, str] = {}
    for path in handler_dir.glob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        for match in HANDLER_OPERATION.finditer(path.read_text()):
            operation = match.group("operation")
            if operation in annotated:
                fail(f"duplicate handler @ID {operation}")
            annotated[operation] = match.group("method")
    if set(annotated) != operations:
        missing = sorted(operations - set(annotated))
        extra = sorted(set(annotated) - operations)
        fail(f"handler @ID values drift from OpenAPI (missing={missing}, extra={extra})")


def operation_routes(document: dict) -> set[tuple[str, str]]:
    return {
        (method.upper(), "/api/v1" + path)
        for path, item in document.get("paths", {}).items()
        if isinstance(item, dict)
        for method, operation in item.items()
        if method != "parameters" and isinstance(operation, dict)
    }


def check_runtime_routes(document: dict, server_path: Path, route_dir: Path) -> None:
    try:
        source = server_path.read_text()
    except OSError as exc:
        fail(f"cannot read console server mount contract {server_path}: {exc}")
    missing = sorted(
        name
        for name in ROUTE_REGISTRATIONS
        if not re.search(rf"\b{re.escape(name)}\(\s*[A-Za-z_]\w*\s*,\s*apiHandler\s*\)", source)
    )
    if missing:
        fail(f"console server is missing route registrations: {', '.join(missing)}")
    if "ogenServer" in source:
        fail("console server must not mount the retired Ogen router")
    registered = {
        (match.group("method"), match.group("path"))
        for path in route_dir.glob("*_http.go")
        for match in RUNTIME_ROUTE.finditer(path.read_text())
    }
    missing_routes = sorted(operation_routes(document) - registered)
    if missing_routes:
        fail(f"console runtime routes drift from OpenAPI (missing={missing_routes})")


def check_api_index(document: dict) -> None:
    boundaries = document.get("x-contract-boundaries")
    if not isinstance(boundaries, dict):
        fail("API index is missing x-contract-boundaries")
    console = boundaries.get("console")
    if (
        not isinstance(console, dict)
        or console.get("source") != "../internal/console/openapi_bundle.json"
        or console.get("generatedSwagger") != "./swagger/swagger.yaml"
        or console.get("routeCount") != CONSOLE_OPERATION_COUNT
    ):
        fail("console contract index must reference the complete Swag-derived route inventory")
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
    if not isinstance(schema, dict) or not schema:
        fail(f"{location} is missing schema")


def is_schema_ref(schema: object) -> bool:
    if not isinstance(schema, dict):
        return False
    if set(schema) == {"$ref"} and isinstance(schema["$ref"], str):
        return True
    all_of = schema.get("allOf")
    return set(schema) == {"allOf"} and isinstance(all_of, list) and any(
        isinstance(part, dict) and isinstance(part.get("$ref"), str) for part in all_of
    )


def check_operation_bodies(operation: dict, location: str) -> None:
    """Require contract-owned schemas at the HTTP operation boundary.

    Request bodies and successful response bodies must reference a named schema,
    either directly or through Swag's response-envelope ``allOf`` composition.
    That keeps an accidental free-form endpoint from silently becoming ``any``.
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


def check_free_form_objects(node: object, location: str) -> None:
    """Allow raw JSON only for admission resource snapshots.

    Contracts for all other objects must expose their fields or, for typed maps,
    declare the value schema through ``additionalProperties: { ... }``.
    """
    if isinstance(node, dict):
        free_form = node.get("additionalProperties") is True or (
            node.get("type") == "object"
            and not any(key in node for key in ("properties", "additionalProperties"))
        )
        if free_form and not location.endswith((".metadataSnapshot", ".specSnapshot")):
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
    console_path = (ROOT / "internal/console/openapi_bundle.json").resolve()
    index_path = (ROOT / "api/openapi.yaml").resolve()
    console = load(console_path)
    index = load(index_path)
    if console.get("openapi") != "3.0.0" or index.get("openapi") != "3.1.0":
        fail("Console contract must use OpenAPI 3.0.0 and the API index must use OpenAPI 3.1.0")
    if "BearerAuth" not in console.get("components", {}).get("securitySchemes", {}):
        fail("console contract must declare BearerAuth")
    console_count = check_operations(console, "console")
    operations = operation_ids(console)
    check_console_route_inventory(operations)
    check_committed_inventory(console, ROOT / "api/console/operation-inventory.json")
    check_handler_coverage(operations, ROOT / "internal/console/handler")
    check_runtime_routes(console, ROOT / "internal/console/server.go", ROOT / "internal/console")
    check_api_index(index)
    check_gateway_proto()
    check_crd_contracts()
    documents = {console_path: console, index_path: index}
    checked: set[Path] = set()
    while unchecked := set(documents) - checked:
        source_path = unchecked.pop()
        check_references(documents[source_path], source_path, documents)
        checked.add(source_path)
    check_free_form_objects(console, "console")
    print(f"API contract OK: {console_count} console operations, {len(GATEWAY_METHODS)} gateway RPCs, {len(CRD_NAMES)} member-cluster CRDs")


if __name__ == "__main__":
    main()
