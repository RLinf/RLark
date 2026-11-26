"""Guard the cross-cluster example against the Kubernetes runtime contract.

The Kubernetes runtime only supports the "rlinf.io/ray" workload, and the Job
controller always rewrites workload.parameters so the main container runs
"rlark-tools ray-launcher" as its primary process. A Pod template "command" is
therefore silently replaced, the workload image must ship the Ray CLI, and the
workload command must live in workload.parameters.run.inline. Only the Ray head
runs that script, after its own Ray cluster reaches parameters.totalNodes nodes.

Run: python3 -m unittest discover -s apps/rlark/docs/examples/tests
"""

from pathlib import Path
import unittest

import yaml


EXAMPLES = Path(__file__).resolve().parents[1]
MANIFEST = EXAMPLES / "cross-cluster-ping.yaml"
RAY_IMAGE = "localhost:5555/ray:quickstart"


def load_jobs():
    with MANIFEST.open() as handle:
        return [doc for doc in yaml.safe_load_all(handle) if doc]


class CrossClusterExampleTest(unittest.TestCase):
    def setUp(self):
        self.jobs = load_jobs()

    def test_every_task_is_a_single_node_ray_head(self):
        # totalNodes is the sum of task replicas in a Job, and a head blocks
        # until that many nodes join. One single-replica task per Job keeps
        # totalNodes at 1 so the head never waits for a peer that the example
        # does not create.
        self.assertTrue(self.jobs)
        for job in self.jobs:
            tasks = job["spec"]["tasks"]
            self.assertEqual(len(tasks), 1, job["metadata"]["name"])
            task = tasks[0]
            self.assertTrue(task["head"], job["metadata"]["name"])
            workload = task["workload"]
            self.assertEqual(workload["kind"], "rlinf.io/ray")
            self.assertEqual(workload["version"], "v1")
            self.assertEqual(workload["carrier"]["parameters"]["replicas"], 1)

    def test_no_task_relies_on_a_pod_template_command(self):
        for job in self.jobs:
            for container in self.containers(job):
                self.assertNotIn("command", container, job["metadata"]["name"])
                self.assertNotIn("args", container, job["metadata"]["name"])

    def test_workload_images_ship_the_ray_cli(self):
        for job in self.jobs:
            for container in self.containers(job):
                self.assertEqual(container["image"], RAY_IMAGE)

    def test_the_serving_side_uses_an_inline_run_script(self):
        servers = [
            job
            for job in self.jobs
            if "run" in job["spec"]["tasks"][0]["workload"].get("parameters", {})
        ]
        self.assertEqual(len(servers), 1)
        script = servers[0]["spec"]["tasks"][0]["workload"]["parameters"]["run"]["inline"]
        self.assertIn("http.server", script)

    def test_the_two_sides_are_placed_in_different_clusters(self):
        clusters = {
            job["spec"]["tasks"][0]["placement"]["nodeSelector"]["rlark.io/cluster-id"]
            for job in self.jobs
        }
        self.assertEqual(len(clusters), len(self.jobs))

    def test_both_sides_share_one_domain(self):
        domains = {job["spec"]["domain"] for job in self.jobs}
        self.assertEqual(len(domains), 1)

    @staticmethod
    def containers(job):
        return job["spec"]["tasks"][0]["workload"]["template"]["spec"]["containers"]


class QuickstartScriptTest(unittest.TestCase):
    def test_scripts_publish_the_ray_workload_image(self):
        for name in ("quickstart.sh", "quickstart-cp.sh"):
            script = (EXAMPLES / name).read_text()
            self.assertIn("rayproject/ray:$RAY_TAG", script, name)
            self.assertIn(f"docker push {RAY_IMAGE}", script, name)


if __name__ == "__main__":
    unittest.main()
