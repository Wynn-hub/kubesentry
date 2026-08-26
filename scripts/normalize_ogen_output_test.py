#!/usr/bin/env python3
"""Regression tests for deterministic ogen output canonicalization."""

import importlib.util
from pathlib import Path
import tempfile
import unittest


NORMALIZER_PATH = Path(__file__).with_name("normalize_ogen_output.py")
SPEC = importlib.util.spec_from_file_location("normalize_ogen_output", NORMALIZER_PATH)
assert SPEC and SPEC.loader
normalizer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(normalizer)


class NormalizeOgenOutputTest(unittest.TestCase):
    def test_removes_blank_only_lines_from_generated_go_files(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            generated = output / "oas_router_gen.go"
            generated.write_text("package dto\n\n \t\nfunc Generated() {}\n")
            normalizer.normalize_directory(output)
            self.assertEqual(generated.read_text(), "package dto\nfunc Generated() {}\n")

    def test_leaves_handwritten_files_unchanged(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            handwritten = output / "error_compat.go"
            handwritten.write_text("package dto\n\nfunc Compatibility() {}\n")
            normalizer.normalize_directory(output)
            self.assertEqual(handwritten.read_text(), "package dto\n\nfunc Compatibility() {}\n")

    def test_ungroups_generated_type_declarations(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            generated = output / "oas_schemas_gen.go"
            generated.write_text("package dto\n\ntype (\n\tOne string\n\tTwo struct{}\n)\n")
            normalizer.normalize_directory(output)
            self.assertEqual(generated.read_text(), "package dto\ntype One string\ntype Two struct{}\n")


if __name__ == "__main__":
    unittest.main()
