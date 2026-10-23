"""Exercise the data-plane script without Docker, Kubernetes, or network access.

Run: python3 -m unittest discover -s apps/rlark/docs/examples/tests
"""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


EXAMPLES = Path(__file__).resolve().parents[1]
STUB = r'''
import base64, json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ["CALL_LOG"], "a") as log:
    log.write(json.dumps([name, args]) + "\n")
failure = os.environ.get("FAILURE", "")
if name == "kubectl":
    if "secret" in args and "get" in args:
        if failure == "secret":
            sys.exit(1)
        print(json.dumps({"data": {"admin-password": base64.b64encode(b'password"with\\quotes').decode()}}))
    elif "nodes" in args and "get" in args:
        print("test-node")
elif name == "docker" and args[0] == "inspect":
    print("test-network" if args[1] == "kcp" else "172.17.0.2")
elif name == "kind" and args[:2] == ["get", "kubeconfig"]:
    print("test-kubeconfig")
elif name == "curl":
    url = next(a for a in args if a.startswith("http://"))
    if url.endswith("/login"):
        assert json.load(sys.stdin) == {"username": "admin", "password": 'password"with\\quotes'}
        if failure == "login":
            sys.exit(22)
        print(json.dumps({"role": "admin", "token": "test-token"}))
    else:
        assert "Authorization: Bearer test-token" in args, "missing authentication"
        if url.endswith("/certificates/agent"):
            assert json.loads(args[args.index("-d") + 1]) == {"cluster_id": "my-cluster"}
            if failure == "certificate":
                sys.exit(22)
            print(json.dumps({} if failure == "incomplete" else {
                "ca_cert": "ca", "agent_cert": "cert", "agent_key": "key"}))
        else:
            if failure == "nodes":
                sys.exit(22)
            print(json.dumps({"items": [{"metadata": {"name": "test-node", "labels": {
                "rlark.io/cluster-id": "rlark-my-cluster"}}}]}))
'''


@unittest.skipUnless(shutil.which("jq"), "jq is required")
class QuickstartDataPlaneTests(unittest.TestCase):
    def run_script(self, failure="", token=None):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            bin_dir = root / "bin"
            bin_dir.mkdir()
            for name in ("docker", "kind", "kubectl", "curl", "sleep"):
                stub = bin_dir / name
                stub.write_text(f"#!{sys.executable}\n" + STUB)
                stub.chmod(0o755)
            runtime = root / "runtime"
            (runtime / "rlark").mkdir(parents=True)
            for name in ("agent-rbac.yaml", "agent-deploy.yaml"):
                shutil.copy(EXAMPLES / name, root / name)
            script = root / "quickstart-dp.sh"
            # Redirect only scratch paths; run the actual script and all its stages.
            script.write_text((EXAMPLES / script.name).read_text().replace("/tmp/", f"{runtime}/"))
            log = root / "calls.jsonl"
            env = dict(os.environ, PATH=f"{bin_dir}:{os.environ['PATH']}",
                       CALL_LOG=str(log), FAILURE=failure)
            env.pop("RLARK_TOKEN", None)
            if token:
                env["RLARK_TOKEN"] = token
            result = subprocess.run(
                ["bash", str(script), "--cluster-id", "my-cluster"],
                env=env, capture_output=True, text=True, timeout=20,
            )
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            return result, calls

    def test_login_and_all_authenticated_requests(self):
        result, calls = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        urls = [next(a for a in args if a.startswith("http://"))
                for name, args in calls if name == "curl"]
        self.assertEqual(sum(url.endswith("/nodes") for url in urls), 3)
        self.assertTrue(any(url.endswith("/certificates/agent") for url in urls))
        self.assertNotIn("test-token", result.stdout + result.stderr)

    def test_existing_token_skips_password_lookup_and_login(self):
        result, calls = self.run_script(token="test-token")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(any(name == "kubectl" and "get" in args and "secret" in args
                             for name, args in calls))
        self.assertFalse(any("http://localhost:9000/api/v1/auth/login" in args
                             for _, args in calls))

    def test_authentication_failures_precede_cluster_mutations(self):
        for failure in ("secret", "login", "nodes"):
            with self.subTest(failure=failure):
                result, calls = self.run_script(failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(name in ("kind", "docker") for name, _ in calls))

    def test_certificate_errors_stop_before_agent_deployment(self):
        for failure in ("certificate", "incomplete"):
            with self.subTest(failure=failure):
                result, calls = self.run_script(failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(name == "kubectl" and "apply" in args
                                     for name, args in calls))


if __name__ == "__main__":
    unittest.main()
