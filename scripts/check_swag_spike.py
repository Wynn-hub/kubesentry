#!/usr/bin/env python3
"""Compare the GET /policies Swag spike with the reviewed OAS 3.1 contract."""

import argparse
import json
from pathlib import Path
import sys


SCHEMA_KEYS = ("type", "format", "enum", "default", "minimum", "maximum", "pattern", "nullable")


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


def schema_contract(document: dict, value: object) -> object:
    value = resolve(document, value)
    if not isinstance(value, dict):
        return value
    if isinstance(value.get("allOf"), list):
        base = {key: child for key, child in value.items() if key != "allOf"}
        parts = [schema_contract(document, child) for child in value["allOf"]]
        parts.append(schema_contract(document, base))
        return merge_contracts([part for part in parts if isinstance(part, dict)])
    contract = {key: value[key] for key in SCHEMA_KEYS if key in value}
    if isinstance(value.get("required"), list):
        contract["required"] = sorted(value["required"])
    if isinstance(value.get("properties"), dict):
        contract["properties"] = {
            name: schema_contract(document, child)
            for name, child in sorted(value["properties"].items())
        }
    if "items" in value:
        contract["items"] = schema_contract(document, value["items"])
    if "additionalProperties" in value:
        child = value["additionalProperties"]
        contract["additionalProperties"] = schema_contract(document, child) if isinstance(child, dict) else child
    for key in ("oneOf", "anyOf"):
        if isinstance(value.get(key), list):
            contract[key] = [schema_contract(document, child) for child in value[key]]
    return contract


def parameter_contract(document: dict, parameter: object) -> dict:
    parameter = resolve(document, parameter)
    if not isinstance(parameter, dict):
        raise ValueError("parameter is not an object")
    return {
        "name": parameter.get("name"),
        "in": parameter.get("in"),
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


def check_spike(old: dict, new: dict) -> None:
    old_operation = old["paths"]["/policies"]["get"]
    new_operation = new["paths"]["/policies"]["get"]
    if old_operation["operationId"] != new_operation["operationId"]:
        raise ValueError("operationId differs")
    old_parameters = [parameter_contract(old, value) for value in old_operation["parameters"]]
    new_parameters = [parameter_contract(new, value) for value in new_operation["parameters"]]
    if old_parameters != new_parameters:
        raise ValueError("parameters differ")
    if set(old_operation["responses"]) != set(new_operation["responses"]):
        raise ValueError("response statuses differ")
    for status in old_operation["responses"]:
        old_schema = response_schema(old, old_operation["responses"][status])
        new_schema = response_schema(new, new_operation["responses"][status])
        if old_schema != new_schema:
            raise ValueError(f"response schema differs for {status}")
    if keyword_counts(old) != keyword_counts(new):
        raise ValueError("nullable/union keyword inventory differs")


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
        check_spike(baseline, candidate)
        check_resource_id_patterns(baseline, candidate)
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"Swag spike contract mismatch: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
    print(f"Swag spike contract OK: GET /policies; nullable/union={keyword_counts(baseline)}")


if __name__ == "__main__":
    main()
