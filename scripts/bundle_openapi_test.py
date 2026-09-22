#!/usr/bin/env python3
"""Regression tests for the derived Console OpenAPI document."""

import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class BundleOpenAPITest(unittest.TestCase):
    def test_generator_writes_normalized_self_contained_document(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "openapi.json"
            result = subprocess.run(
                [sys.executable, "scripts/bundle_openapi.py", "--output", str(output)],
                cwd=ROOT,
                capture_output=True,
                text=True,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            document = json.loads(output.read_text())
            self.assertEqual(document["openapi"], "3.0.0")
            self.assertEqual(operation_count(document), 50)
            self.assertIn("BearerAuth", document["components"]["securitySchemes"])
            assert_internal_refs_resolve(self, document, document)
            self.assertEqual(
                output.read_text(),
                json.dumps(document, indent=4, sort_keys=True, ensure_ascii=False) + "\n",
            )


def operation_count(document: dict) -> int:
    return sum(
        1
        for item in document["paths"].values()
        for method in item
        if method != "parameters"
    )


def assert_internal_refs_resolve(test: unittest.TestCase, root: object, value: object) -> None:
    if isinstance(value, dict):
        ref = value.get("$ref")
        if isinstance(ref, str):
            test.assertTrue(ref.startswith("#/"), ref)
            target = root
            for segment in ref[2:].split("/"):
                segment = segment.replace("~1", "/").replace("~0", "~")
                test.assertIsInstance(target, dict, ref)
                test.assertIn(segment, target, ref)
                target = target[segment]
        for child in value.values():
            assert_internal_refs_resolve(test, root, child)
    elif isinstance(value, list):
        for child in value:
            assert_internal_refs_resolve(test, root, child)


if __name__ == "__main__":
    unittest.main()
