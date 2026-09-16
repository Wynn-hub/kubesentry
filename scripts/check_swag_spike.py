#!/usr/bin/env python3
"""Compare migrated Swag operations with the reviewed OAS 3.1 contract."""

import argparse
import json
from pathlib import Path
import sys


SCHEMA_KEYS = (
    "type",
    "format",
    "enum",
    "default",
    "minimum",
    "maximum",
    "exclusiveMinimum",
    "exclusiveMaximum",
    "minLength",
    "maxLength",
    "minItems",
    "maxItems",
    "uniqueItems",
    "multipleOf",
    "pattern",
    "nullable",
)
HTTP_METHODS = {"delete", "get", "head", "options", "patch", "post", "put", "trace"}
EXPECTED_MIGRATED_OPERATION_IDS = {
    "getAdmissionEvent",
    "getAdmissionEventSettings",
    "getAdmissionEventTrend",
    "getAdmissionLinkIncident",
    "createCluster",
    "createDistribution",
    "createException",
    "createUninstallTask",
    "deleteCluster",
    "deleteDistribution",
    "deleteException",
    "getCluster",
    "getDistribution",
    "getException",
    "getFieldSchema",
    "getLatestInstallParams",
    "getPlatformInstanceTrend",
    "getPlatformOverview",
    "getPlatformSettings",
    "getSummary",
    "listClusters",
    "listAdmissionEvents",
    "listAdmissionLinkIncidents",
    "listDistributions",
    "listExceptions",
    "listInstallRecords",
    "listPlatformLeadershipEvents",
    "listPolicies",
    "listUninstallTasks",
    "reissueClusterToken",
    "retryUninstallTask",
    "updateCluster",
    "updateAdmissionEventSettings",
    "updateDistribution",
    "updateException",
    "updatePlatformSettings",
}


def keyword_counts(node: object) -> dict[str, int]:
    counts = {key: 0 for key in ("anyOf", "nullable", "oneOf")}
    if isinstance(node, dict):
        for key, value in node.items():
            if key in counts:
                counts[key] += 1
            child_counts = keyword_counts(value)
            for child_key, count in child_counts.items():
                counts[child_key] += count
    elif isinstance(node, list):
        for value in node:
            child_counts = keyword_counts(value)
            for child_key, count in child_counts.items():
                counts[child_key] += count
    return counts


def resolve(document: dict, value: object) -> object:
    seen: set[str] = set()
    while isinstance(value, dict) and isinstance(value.get("$ref"), str):
        ref = value["$ref"]
        if not ref.startswith("#/") or ref in seen:
            raise ValueError(f"unsupported reference {ref}")
        seen.add(ref)
        target: object = document
        for part in ref[2:].split("/"):
            part = part.replace("~1", "/").replace("~0", "~")
            target = target[part]  # type: ignore[index]
        value = target
    return value


def merge_contracts(parts: list[dict]) -> dict:
    merged: dict = {}
    for part in parts:
        for key, value in part.items():
            if key == "properties":
                merged.setdefault(key, {}).update(value)
            elif key == "required":
                merged[key] = sorted(set(merged.get(key, [])) | set(value))
            elif key not in merged:
                merged[key] = value
            elif merged[key] != value:
                raise ValueError(f"incompatible allOf value for {key}")
    return merged


def schema_contract(document: dict, value: object, ref_stack: tuple[str, ...] = ()) -> object:
    if isinstance(value, dict) and isinstance(value.get("$ref"), str):
        ref = value["$ref"]
        if ref in ref_stack:
            return {"$recursive": len(ref_stack) - ref_stack.index(ref)}
        return schema_contract(document, resolve(document, value), ref_stack + (ref,))
    if not isinstance(value, dict):
        return value
    if isinstance(value.get("allOf"), list):
        base = {key: child for key, child in value.items() if key != "allOf"}
        parts = [schema_contract(document, child, ref_stack) for child in value["allOf"]]
        parts.append(schema_contract(document, base, ref_stack))
        return merge_contracts([part for part in parts if isinstance(part, dict)])
    contract = {key: value[key] for key in SCHEMA_KEYS if key in value}
    if isinstance(value.get("required"), list):
        contract["required"] = sorted(value["required"])
    if isinstance(value.get("properties"), dict):
        contract["properties"] = {
            name: schema_contract(document, child, ref_stack)
            for name, child in sorted(value["properties"].items())
        }
    if "items" in value:
        contract["items"] = schema_contract(document, value["items"], ref_stack)
    if "additionalProperties" in value:
        child = value["additionalProperties"]
        if child is not True:
            contract["additionalProperties"] = schema_contract(document, child, ref_stack) if isinstance(child, dict) else child
    for key in ("oneOf", "anyOf"):
        if isinstance(value.get(key), list):
            contract[key] = [schema_contract(document, child, ref_stack) for child in value[key]]
    return contract


def parameter_contract(document: dict, parameter: object) -> dict:
    parameter = resolve(document, parameter)
    if not isinstance(parameter, dict):
        raise ValueError("parameter is not an object")
    return {
        "name": parameter.get("name"),
        "in": parameter.get("in"),
        "described": bool(parameter.get("description")),
        "required": parameter.get("required", False),
        "style": parameter.get("style"),
        "explode": parameter.get("explode"),
        "schema": schema_contract(document, parameter.get("schema", {})),
    }


def response_schema(document: dict, response: object) -> object:
    response = resolve(document, response)
    if not isinstance(response, dict):
        raise ValueError("response is not an object")
    media = response.get("content", {}).get("application/json", {})
    return schema_contract(document, media.get("schema", {}))


def check_parameters(old_parameters: list[dict], new_parameters: list[dict]) -> None:
    if len(old_parameters) != len(new_parameters):
        raise ValueError("parameters differ")
    for old, new in zip(old_parameters, new_parameters):
        old = dict(old)
        new = dict(new)
        old_described = old.pop("described")
        new_described = new.pop("described")
        if old != new or old_described and not new_described:
            raise ValueError("parameters differ")


def request_body_contract(document: dict, operation: dict) -> object:
    if "requestBody" not in operation:
        return None
    body = resolve(document, operation["requestBody"])
    if not isinstance(body, dict):
        raise ValueError("request body is not an object")
    media = body.get("content", {}).get("application/json", {})
    return {
        "required": body.get("required", False),
        "schema": schema_contract(document, media.get("schema", {})),
    }


def check_operation(old: dict, new: dict, path: str, method: str) -> None:
    old_operation = old["paths"][path][method]
    new_operation = new["paths"][path][method]
    if old_operation["operationId"] != new_operation["operationId"]:
        raise ValueError("operationId differs")
    old_values = old["paths"][path].get("parameters", []) + old_operation.get("parameters", [])
    new_values = new["paths"][path].get("parameters", []) + new_operation.get("parameters", [])
    old_parameters = [parameter_contract(old, value) for value in old_values]
    new_parameters = [parameter_contract(new, value) for value in new_values]
    check_parameters(old_parameters, new_parameters)
    if request_body_contract(old, old_operation) != request_body_contract(new, new_operation):
        raise ValueError("request body differs")
    if set(old_operation["responses"]) != set(new_operation["responses"]):
        raise ValueError("response statuses differ")
    for status in old_operation["responses"]:
        old_schema = response_schema(old, old_operation["responses"][status])
        new_schema = response_schema(new, new_operation["responses"][status])
        if old_schema != new_schema:
            raise ValueError(f"response schema differs for {status}")


def check_migrated_operations(old: dict, new: dict) -> int:
    check_operation_set(new, EXPECTED_MIGRATED_OPERATION_IDS)
    count = 0
    for path, path_item in new["paths"].items():
        for method in path_item:
            if method not in HTTP_METHODS:
                continue
            try:
                check_operation(old, new, path, method)
            except (KeyError, ValueError) as exc:
                raise ValueError(f"{method.upper()} {path}: {exc}") from exc
            count += 1
    return count


def check_operation_set(document: dict, expected: set[str]) -> None:
    actual = {
        operation["operationId"]
        for path_item in document["paths"].values()
        for method, operation in path_item.items()
        if method in HTTP_METHODS
    }
    if actual != expected:
        missing = sorted(expected - actual)
        unexpected = sorted(actual - expected)
        raise ValueError(f"operation set differs: missing={missing}, unexpected={unexpected}")


def check_resource_id_patterns(old: dict, new: dict) -> None:
    expected = old["components"]["schemas"]["ResourceID"]["pattern"]
    fields = (
        ("dto.UninstallTaskHTTPView", "id"),
        ("dto.UninstallTaskIDView", "id"),
        ("dto.PlatformLeadershipEventView", "id"),
        ("dto.PlatformLeadershipEventView", "clusterId"),
    )
    for schema_name, field_name in fields:
        actual = new["components"]["schemas"][schema_name]["properties"][field_name].get("pattern")
        if actual != expected:
            raise ValueError(f"{schema_name}.{field_name} pattern differs")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("baseline", type=Path)
    parser.add_argument("candidate", type=Path)
    args = parser.parse_args()
    try:
        baseline = json.loads(args.baseline.read_text())
        candidate = json.loads(args.candidate.read_text())
        operation_count = check_migrated_operations(baseline, candidate)
        check_resource_id_patterns(baseline, candidate)
        if keyword_counts(baseline) != keyword_counts(candidate):
            raise ValueError("nullable/union keyword inventory differs")
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"Swag spike contract mismatch: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
    print(f"Swag contract OK: {operation_count} operation(s); nullable/union={keyword_counts(baseline)}")


if __name__ == "__main__":
    main()
