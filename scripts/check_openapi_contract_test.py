#!/usr/bin/env python3
"""Regression tests for the repository-local OpenAPI contract gate."""

import contextlib
import importlib.util
import io
from pathlib import Path
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


if __name__ == "__main__":
    unittest.main()
