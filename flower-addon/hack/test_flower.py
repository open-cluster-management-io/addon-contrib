"""Check rendered deployments and app bundles against the installed Flower release."""

from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import yaml
from flwr.supercore.cli.flower_superexec import _parse_args as superexec_parser
from flwr.superlink.cli.flower_superlink import _parse_args_run_superlink
from flwr.supernode.cli.flower_supernode import _parse_args_run_supernode as supernode_parser


ADDON = Path(__file__).resolve().parents[1]
ROOT = ADDON.parent


class FlowerCompatibilityTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        rendered = subprocess.check_output(
            ["helm", "template", "flower-addon", str(ADDON / "charts/flower-addon")],
            text=True,
        )
        cls.resources = [r for r in yaml.safe_load_all(rendered) if r]

    def test_shared_http_listener(self):
        deployment = next(r for r in self.resources if r["kind"] == "Deployment")
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        args = _parse_args_run_superlink().parse_args(container["args"])
        self.assertEqual(args.host, "0.0.0.0")
        self.assertEqual(args.isolation, "process")
        port = container["ports"][0]
        self.assertEqual(port["containerPort"], args.port)
        for probe in ("readinessProbe", "livenessProbe"):
            self.assertEqual(container[probe]["tcpSocket"]["port"], port["name"])
        service = next(r for r in self.resources if r["kind"] == "Service")
        self.assertEqual(len(service["spec"]["ports"]), 3)
        for exposed in service["spec"]["ports"]:
            self.assertEqual(exposed["targetPort"], port["name"])

    def test_supernode_runtime_listener(self):
        template = next(r for r in self.resources if r["kind"] == "AddOnTemplate")
        deployment = next(
            r for r in template["spec"]["agentSpec"]["workload"]["manifests"]
            if r["kind"] == "Deployment"
        )
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        args = supernode_parser().parse_args(container["args"])
        self.assertEqual(args.host, "0.0.0.0")
        self.assertEqual(args.port, container["ports"][0]["containerPort"])
        self.assertEqual(args.isolation, "process")

    def test_superexec_commands(self):
        paths = list((ADDON / "cifar10/deploy").glob("superexec-*.yaml"))
        paths += list((ROOT / "federated-learning-controller/internal/controller/manifests/flower").rglob("*.yaml"))
        for path in paths:
            with self.subTest(path=path):
                args = re.findall(r'"(--[^"\n]+)"', path.read_text())
                if not args:
                    continue
                self.assertTrue(any(a.startswith("--runtime-api-address=") for a in args))
                with patch.object(sys, "argv", ["flower-superexec", *args]):
                    superexec_parser().parse_args()

    def test_app_bundles(self):
        apps = [ADDON / "cifar10", ROOT / "federated-learning-controller/examples/flower/app-torch"]
        with tempfile.TemporaryDirectory() as directory:
            for app in apps:
                with self.subTest(app=app):
                    subprocess.run(
                        [str(Path(sys.executable).with_name("flwr")), "build", "--app", str(app)],
                        cwd=directory,
                        check=True,
                    )


if __name__ == "__main__":
    unittest.main()
