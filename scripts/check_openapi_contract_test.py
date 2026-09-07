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

    def test_rejects_parameter_without_schema_metadata(self):
        self.assert_contract_error(
            lambda: checker.check_parameter_metadata(
                {"name": "health", "in": "query", "description": "健康状态筛选", "schema": {"type": "string"}},
                "console GET /platform/overview parameter health",
            ),
            "parameter schema is missing description",
        )

    def test_platform_runtime_stats_and_leadership_roles_are_explicit(self):
        platform = checker.load(checker.ROOT / "api/schemas/platform.yaml")
        schemas = platform["components"]["schemas"]
        roles = schemas["LeadershipEvent"]["properties"]
        self.assertEqual(roles["roleFrom"]["enum"], ["leader", "standby", "none"])
        self.assertEqual(roles["roleTo"]["enum"], ["leader", "standby", "none"])
        stats = schemas["DatabaseStats"]["properties"]
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

    def test_rejects_generated_handler_that_leaves_a_documented_operation_unimplemented(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            generated = root / "oas_server_gen.go"
            generated.write_text("// ListWidgets implements listWidgets operation.\n")
            handlers = root / "handler"
            handlers.mkdir()
            (handlers / "widgets.go").write_text("package handler\n")
            self.assert_contract_error(
                lambda: checker.check_generated_handler_coverage({"listWidgets"}, generated, handlers),
                "have no concrete handler method",
            )

    def test_ignores_handler_methods_declared_only_in_test_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            generated = root / "oas_server_gen.go"
            generated.write_text("// ListWidgets implements listWidgets operation.\n")
            handlers = root / "handler"
            handlers.mkdir()
            (handlers / "widgets_test.go").write_text(
                "package handler\nfunc (h *Handler) ListWidgets() {}\n"
            )
            self.assert_contract_error(
                lambda: checker.check_generated_handler_coverage({"listWidgets"}, generated, handlers),
                "have no concrete handler method",
            )

    def test_rejects_server_without_the_generated_api_prefix_mount(self):
        with tempfile.TemporaryDirectory() as directory:
            server = Path(directory) / "server.go"
            server.write_text('mux.Handle("/api/v1/clusters", ogenServer)\n')
            self.assert_contract_error(
                lambda: checker.check_runtime_mount(server),
                "must mount the generated server at /api/v1/",
            )

    def test_accepts_generated_server_mounted_through_json_404_wrapper(self):
        with tempfile.TemporaryDirectory() as directory:
            server = Path(directory) / "server.go"
            server.write_text(
                'mux.Handle("/api/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n'
                "rec := httptest.NewRecorder()\n"
                "ogenServer.ServeHTTP(rec, r)\n"
                "}))\n"
            )
            checker.check_runtime_mount(server)

    def test_rejects_generated_server_call_outside_api_prefix_handler(self):
        with tempfile.TemporaryDirectory() as directory:
            server = Path(directory) / "server.go"
            server.write_text(
                'mux.Handle("/api/v1/", otherHandler)\n'
                "func unused() { ogenServer.ServeHTTP(nil, nil) }\n"
            )
            self.assert_contract_error(
                lambda: checker.check_runtime_mount(server),
                "must mount the generated server at /api/v1/",
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
