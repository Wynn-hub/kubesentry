#!/usr/bin/env python3
"""Tests for repository-local Go package boundary checks."""

import importlib.util
from pathlib import Path
import tempfile
import unittest


CHECKER_PATH = Path(__file__).with_name("check_architecture_boundaries.py")


def load_checker():
    spec = importlib.util.spec_from_file_location("check_architecture_boundaries", CHECKER_PATH)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ArchitectureBoundaryTest(unittest.TestCase):
    def test_accepts_allowed_protocol_and_public_api_imports(self):
        checker = load_checker()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            hublink = root / "internal/operator/hublink/client.go"
            hublink.parent.mkdir(parents=True)
            hublink.write_text(
                'package hublink\nimport "github.com/Wynn-hub/kubesentry/internal/gatewaypb"\n'
            )
            public_api = root / "api/kubesentry/v1alpha1/types.go"
            public_api.parent.mkdir(parents=True)
            public_api.write_text('package v1alpha1\nimport "k8s.io/apimachinery/pkg/runtime"\n')

            self.assertEqual(checker.violations(root), [])

    def test_rejects_legacy_cross_plane_and_public_internal_imports(self):
        checker = load_checker()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            legacy = root / "internal/example/example.go"
            legacy.parent.mkdir(parents=True)
            legacy.write_text(
                'package example\nimport "github.com/Wynn-hub/kubesentry/internal/dbrepo"\n'
            )
            hublink = root / "internal/operator/hublink/client.go"
            hublink.parent.mkdir(parents=True)
            hublink.write_text(
                'package hublink\nimport "github.com/Wynn-hub/kubesentry/internal/gateway"\n'
            )
            public_api = root / "api/kubesentry/v1alpha1/types.go"
            public_api.parent.mkdir(parents=True)
            public_api.write_text(
                'package v1alpha1\nimport "github.com/Wynn-hub/kubesentry/internal/logging"\n'
            )

            got = "\n".join(checker.violations(root))
            self.assertIn("legacy import", got)
            self.assertIn("hublink production import", got)
            self.assertIn("public API import", got)


if __name__ == "__main__":
    unittest.main()
