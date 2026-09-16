#!/usr/bin/env python3
"""Regression tests for strict Swagger 2 to OpenAPI 3 conversion."""

import importlib.util
import json
from pathlib import Path
import stat
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("convert_swagger_to_openapi.py")
SPEC = importlib.util.spec_from_file_location("convert_swagger_to_openapi", SCRIPT)
assert SPEC and SPEC.loader
converter = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(converter)
NORMALIZER_SCRIPT = Path(__file__).with_name("normalize_openapi_json.py")
NORMALIZER_SPEC = importlib.util.spec_from_file_location("normalize_openapi_json", NORMALIZER_SCRIPT)
assert NORMALIZER_SPEC and NORMALIZER_SPEC.loader
normalizer = importlib.util.module_from_spec(NORMALIZER_SPEC)
NORMALIZER_SPEC.loader.exec_module(normalizer)
SPIKE_CHECK_SCRIPT = Path(__file__).with_name("check_swag_spike.py")
SPIKE_CHECK_SPEC = importlib.util.spec_from_file_location("check_swag_spike", SPIKE_CHECK_SCRIPT)
assert SPIKE_CHECK_SPEC and SPIKE_CHECK_SPEC.loader
spike_check = importlib.util.module_from_spec(SPIKE_CHECK_SPEC)
SPIKE_CHECK_SPEC.loader.exec_module(spike_check)


class ConvertSwaggerToOpenAPITest(unittest.TestCase):
    def make_converter(self, directory: Path, stderr: str = "") -> Path:
        executable = directory / "fake-converter"
        executable.write_text(
            "#!/bin/sh\n"
            "set -eu\n"
            f"printf '%s' '{stderr}' >&2\n"
            "while [ \"$#\" -gt 0 ]; do\n"
            "  if [ \"$1\" = --outfile ]; then shift; output=$1; fi\n"
            "  shift\n"
            "done\n"
            "printf '%s\\n' '{\"openapi\":\"3.0.0\"}' > \"$output\"\n"
        )
        executable.chmod(executable.stat().st_mode | stat.S_IXUSR)
        return executable

    def test_rejects_warning_even_when_converter_exits_zero(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "openapi.json"
            fake = self.make_converter(root, "Warning: repaired input")

            with self.assertRaisesRegex(converter.ConversionError, "warning"):
                converter.run_conversion(root / "swagger.yaml", output, fake)

            self.assertFalse(output.exists())

    def test_accepts_silent_success(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "openapi.json"
            fake = self.make_converter(root)

            converter.run_conversion(root / "swagger.yaml", output, fake)

            self.assertEqual(output.read_text(), '{"openapi":"3.0.0"}\n')

    def test_normalizer_sorts_object_keys_without_reordering_arrays(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "raw.json"
            output = root / "normalized.json"
            source.write_text('{"z":[{"b":2,"a":1},0],"a":"first"}')

            normalizer.normalize(source, output)

            self.assertEqual(
                output.read_text(),
                '{\n    "a": "first",\n    "z": [\n        {\n            "a": 1,\n            "b": 2\n        },\n        0\n    ]\n}\n',
            )

    def test_spike_comparison_accepts_all_of_equivalent_to_flat_object(self):
        old = {
            "components": {"schemas": {
                "Envelope": {"type": "object", "required": ["code"], "properties": {"code": {"type": "integer"}}},
                "Response": {"allOf": [
                    {"$ref": "#/components/schemas/Envelope"},
                    {"type": "object", "properties": {"createdAt": {"type": "string", "format": "date-time"}}},
                ]},
            }},
        }
        new = {"type": "object", "required": ["code"], "properties": {
            "code": {"type": "integer"},
            "createdAt": {"type": "string", "format": "date-time"},
        }}

        self.assertEqual(
            spike_check.schema_contract(old, {"$ref": "#/components/schemas/Response"}),
            spike_check.schema_contract({}, new),
        )

    def test_spike_comparison_detects_lost_date_time_format(self):
        formatted = {"type": "string", "format": "date-time"}
        plain = {"type": "string"}
        self.assertNotEqual(
            spike_check.schema_contract({}, formatted),
            spike_check.schema_contract({}, plain),
        )

    def test_operation_set_rejects_removed_operation(self):
        document = {"paths": {"/kept": {"get": {"operationId": "kept"}}}}

        with self.assertRaisesRegex(ValueError, "missing=\\['removed'\\]"):
            spike_check.check_operation_set(document, {"kept", "removed"})

    def test_counts_nullable_and_union_keywords(self):
        document = {"nullable": True, "nested": [{"oneOf": []}, {"anyOf": []}]}
        self.assertEqual(
            spike_check.keyword_counts(document),
            {"anyOf": 1, "nullable": 1, "oneOf": 1},
        )

    def test_strict_converter_preserves_nullable_and_union_extensions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "swagger.json"
            output = root / "openapi.json"
            source.write_text(json.dumps({
                "swagger": "2.0",
                "info": {"title": "semantic probe", "version": "1.0.0"},
                "paths": {},
                "definitions": {
                    "Probe": {
                        "type": "object",
                        "properties": {
                            "nullableValue": {"type": "string", "x-nullable": True},
                            "oneOfValue": {"x-oneOf": [{"type": "string"}, {"type": "integer"}]},
                            "anyOfValue": {"x-anyOf": [{"type": "boolean"}, {"type": "number"}]},
                        },
                    },
                },
            }))

            converter.run_conversion(
                source,
                output,
                Path(__file__).parents[1] / "web/node_modules/.bin/swagger2openapi",
            )
            properties = json.loads(output.read_text())["components"]["schemas"]["Probe"]["properties"]

            self.assertEqual(properties["nullableValue"], {"type": "string", "nullable": True})
            self.assertEqual(properties["oneOfValue"]["oneOf"], [{"type": "string"}, {"type": "integer"}])
            self.assertEqual(properties["anyOfValue"]["anyOf"], [{"type": "boolean"}, {"type": "number"}])


if __name__ == "__main__":
    unittest.main()
