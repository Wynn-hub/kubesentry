#!/usr/bin/env python3
"""Regression tests for the repository-local OpenAPI contract gate."""

import contextlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest


CHECKER_PATH = Path(__file__).with_name("check_openapi_contract.py")
SPEC = importlib.util.spec_from_file_location("check_openapi_contract", CHECKER_PATH)
assert SPEC and SPEC.loader
checker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(checker)


class ContractCheckerTest(unittest.TestCase):
    def assert_contract_error(self, fn, message):
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            with self.assertRaises(SystemExit):
                fn()
        self.assertIn(message, stderr.getvalue())

    def test_rejects_free_form_request_body(self):
        operation = {
            "requestBody": {
                "content": {"application/json": {"schema": {"type": "object", "additionalProperties": True}}}
            },
            "responses": {"200": {"description": "ok"}},
        }
        self.assert_contract_error(
            lambda: checker.check_operation_bodies(operation, "console POST /widgets"),
            "request body must reference an explicit schema",
        )

    def test_rejects_free_form_success_response(self):
        operation = {
            "responses": {
                "200": {
                    "description": "ok",
                    "content": {"application/json": {"schema": {"type": "object", "additionalProperties": True}}},
                }
            }
        }
        self.assert_contract_error(
            lambda: checker.check_operation_bodies(operation, "console GET /widgets"),
            "success response must reference an explicit schema",
        )

    def test_rejects_missing_operation_metadata(self):
        self.assert_contract_error(
            lambda: checker.check_operations({"paths": {"/widgets": {"get": {"responses": {"200": {"description": "ok"}}}}}}, "console"),
            "missing tags",
        )

    def test_rejects_unresolved_local_reference(self):
        self.assert_contract_error(
            lambda: checker.resolve_pointer({"components": {}}, "#/components/schemas/Missing", "console GET /widgets"),
            "unresolved reference",
        )

    def test_rejects_unapproved_free_form_object(self):
        self.assert_contract_error(
            lambda: checker.check_free_form_objects(
                {"type": "object", "additionalProperties": True},
                "platform.yaml.components.schemas.PlatformInstance.properties.dbStats",
            ),
            "unapproved additionalProperties: true",
        )

    def test_rejects_parameter_without_schema(self):
        self.assert_contract_error(
            lambda: checker.check_parameter_metadata(
                {"name": "health", "in": "query", "description": "健康状态筛选"},
                "console GET /platform/overview parameter health",
            ),
            "is missing schema",
        )

    def test_platform_runtime_stats_and_leadership_roles_are_explicit(self):
        console = checker.load(checker.ROOT / "internal/console/openapi_bundle.json")
        schemas = console["components"]["schemas"]
        roles = schemas["dto.PlatformLeadershipEventView"]["properties"]
        self.assertEqual(roles["roleFrom"]["enum"], ["leader", "standby", "none"])
        self.assertEqual(roles["roleTo"]["enum"], ["leader", "standby", "none"])
        stats = schemas["dto.PlatformDatabaseStatsView"]["properties"]
        self.assertEqual(
            set(stats),
            {"connections", "maxConnections", "transactions", "rollbacks", "cacheHitRatio", "databaseBytes", "poolInUse", "poolIdle", "poolWaitCount", "poolWaitSeconds"},
        )

    def test_rejects_console_route_inventory_that_is_not_all_50_operations(self):
        self.assert_contract_error(
            lambda: checker.check_console_route_inventory({"onlyOne"}),
            "expected 50 console operations",
        )

    def test_builds_stable_operation_inventory_with_parameters_and_responses(self):
        document = {
            "paths": {
                "/widgets": {
                    "get": {
                        "operationId": "listWidgets",
                        "parameters": [
                            {"$ref": "#/components/parameters/Page"},
                            {"name": "page", "in": "query", "schema": {"type": "integer"}},
                        ],
                        "responses": {
                            "200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/WidgetList"}}}}
                        },
                    }
                }
            }
        }

        self.assertEqual(
            checker.build_operation_inventory(document),
            [{
                "method": "GET",
                "operationId": "listWidgets",
                "parameters": [
                    {"$ref": "#/components/parameters/Page"},
                    {"in": "query", "name": "page", "required": False, "schema": {"type": "integer"}},
                ],
                "path": "/widgets",
                "responses": {"200": ["#/components/schemas/WidgetList"]},
            }],
        )

    def test_rejects_committed_operation_inventory_drift(self):
        with tempfile.TemporaryDirectory() as directory:
            inventory = Path(directory) / "inventory.json"
            inventory.write_text('{"operationCount": 1, "operations": [], "sha256": "stale"}\n')
            self.assert_contract_error(
                lambda: checker.check_committed_inventory({"paths": {}}, inventory),
                "operation inventory drift",
            )

    def test_rejects_handler_without_a_documented_operation_annotation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            handlers = root / "handler"
            handlers.mkdir()
            (handlers / "widgets.go").write_text("package handler\n")
            self.assert_contract_error(
                lambda: checker.check_handler_coverage({"listWidgets"}, handlers),
                "handler @ID values drift",
            )

    def test_ignores_handler_methods_declared_only_in_test_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            handlers = root / "handler"
            handlers.mkdir()
            (handlers / "widgets_test.go").write_text(
                "package handler\n// @ID listWidgets\nfunc (h *Handler) ListWidgets() {}\n"
            )
            self.assert_contract_error(
                lambda: checker.check_handler_coverage({"listWidgets"}, handlers),
                "handler @ID values drift",
            )

    def test_rejects_server_without_all_route_registrations(self):
        with tempfile.TemporaryDirectory() as directory:
            server = Path(directory) / "server.go"
            server.write_text("registerPolicyRoutes(mux, apiHandler)\n")
            self.assert_contract_error(
                lambda: checker.check_runtime_routes({}, server, Path(directory)),
                "missing route registrations",
            )

    def test_accepts_all_route_registrations(self):
        with tempfile.TemporaryDirectory() as directory:
            server = Path(directory) / "server.go"
            server.write_text("\n".join(f"{name}(mux, apiHandler)" for name in checker.ROUTE_REGISTRATIONS))
            routes = Path(directory) / "widgets_http.go"
            routes.write_text('mux.HandleFunc("GET /api/v1/widgets", nil)\n')
            checker.check_runtime_routes({"paths": {"/widgets": {"get": {}}}}, server, Path(directory))

    def test_rejects_missing_runtime_method_and_path(self):
        with tempfile.TemporaryDirectory() as directory:
            server = Path(directory) / "server.go"
            server.write_text("\n".join(f"{name}(mux, apiHandler)" for name in checker.ROUTE_REGISTRATIONS))
            routes = Path(directory) / "widgets_http.go"
            routes.write_text('mux.HandleFunc("POST /api/v1/widgets", nil)\n')
            self.assert_contract_error(
                lambda: checker.check_runtime_routes({"paths": {"/widgets": {"get": {}}}}, server, Path(directory)),
                "runtime routes drift",
            )

    def test_rejects_retired_ogen_router(self):
        with tempfile.TemporaryDirectory() as directory:
            server = Path(directory) / "server.go"
            source = "\n".join(f"{name}(mux, apiHandler)" for name in checker.ROUTE_REGISTRATIONS)
            server.write_text(source + "\nogenServer.ServeHTTP(nil, nil)\n")
            self.assert_contract_error(
                lambda: checker.check_runtime_routes({}, server, Path(directory)),
                "must not mount the retired Ogen router",
            )

    def test_rejects_api_index_that_treats_gateway_as_openapi(self):
        index = checker.load(checker.ROOT / "api/openapi.yaml")
        index["x-contract-boundaries"]["gateway"]["source"] = "./internal/gateway.yaml"
        self.assert_contract_error(
            lambda: checker.check_api_index(index),
            "gateway contract must be protobuf-first",
        )


if __name__ == "__main__":
    unittest.main()
