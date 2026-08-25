#!/usr/bin/env python3
"""Regression tests for the checked-in Console OpenAPI bundle."""

import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class BundleOpenAPITest(unittest.TestCase):
    def test_generator_writes_self_contained_document(self) -> None:
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
            self.assertEqual(document["openapi"], "3.1.0")
            self.assertEqual(
                {tag["name"] for tag in document["tags"]},
                {"Policy", "PolicyGroup", "Exception", "Cluster", "Distribution", "Platform", "Summary", "AdmissionEvent", "LinkIncident", "Schema"},
            )
            assert_internal_refs_resolve(self, document, document)
            cluster_empty = document["paths"]["/clusters/{name}"]["delete"]["responses"]["200"]["content"]["application/json"]["schema"]["$ref"]
            policy_empty = document["paths"]["/policies/{name}"]["delete"]["responses"]["200"]["content"]["application/json"]["schema"]["$ref"]
            self.assertNotEqual(cluster_empty, policy_empty, "same-named external components must not overwrite each other")


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
