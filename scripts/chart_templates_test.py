import pathlib
import subprocess
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]


class ChartTemplateTest(unittest.TestCase):
    def test_admission_events_service_is_rendered_when_secret_exists(self):
        original = (ROOT / "charts/kubesentry/templates/admission-events.yaml").read_text()
        template = original.replace(
            'lookup "v1" "Secret" .Values.namespace "kubesentry-admission-events"',
            'dict "data" (dict "token" "dGVzdA==")',
        )
        self.assertNotEqual(template, original, "lookup fixture was not injected")

        with tempfile.TemporaryDirectory() as directory:
            chart = pathlib.Path(directory)
            (chart / "templates").mkdir()
            (chart / "Chart.yaml").write_text("apiVersion: v2\nname: test\nversion: 0.1.0\n")
            (chart / "values.yaml").write_text(
                "namespace: kubesentry-system\nadmissionEvents:\n  servicePort: 8082\n"
            )
            (chart / "templates/admission-events.yaml").write_text(template)
            rendered = subprocess.run(
                ["helm", "template", "test", str(chart)],
                check=True,
                capture_output=True,
                text=True,
            ).stdout

        self.assertIn("kind: Service", rendered)
        self.assertIn("name: kubesentry-operator-events", rendered)


if __name__ == "__main__":
    unittest.main()
