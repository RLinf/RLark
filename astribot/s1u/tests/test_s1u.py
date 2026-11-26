"""The S1U robot x86's model configuration (astribot/s1u): the node tuner, the node profile, the Orin and the
clock. Run with pytest from any directory: python3 -m pytest astribot/s1u/tests."""
import hashlib
import importlib.util
import io
import json
import re
import subprocess
import sys
import tarfile
import time
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parents[1]
TUNER, ORIN, CLOCK = HERE / "tuner", HERE / "orin", HERE / "clock"
CATALOG = HERE.parents[1] / "apps/rlark/pkg/addons/catalog/s1u-robot"
for directory in (TUNER, ORIN, CLOCK):
    sys.path.insert(0, str(directory))  # node_tuner.py imports node_spec.py from beside itself
specification = importlib.util.spec_from_file_location("node_tuner", TUNER / "node_tuner.py")
node_tuner = importlib.util.module_from_spec(specification)
specification.loader.exec_module(node_tuner)
import clock  # noqa: E402
import node_spec  # noqa: E402
import orin  # noqa: E402

PROFILE = dict(line.split("=", 1) for line in (TUNER / "node_profile.env").read_text().splitlines()
               if line[:1].isalpha() and "=" in line)


def shell(function):
    """A function of node_profile.sh with the profile's values, on a 20-CPU host."""
    script = (TUNER / "node_profile.sh").read_text()
    functions = script[script.index("cpus() {"):script.index('case "${1:-}" in')]
    return subprocess.run(["bash", "-c", f"nproc() {{ echo 20; }}\n{functions}\nsource {TUNER}/node_profile.env\n{function}"],
                          check=True, capture_output=True, text=True).stdout.strip()


def done(stdout=b"", returncode=0, stderr=b""):
    return type("R", (), {"returncode": returncode, "stdout": stdout, "stderr": stderr})()


def test_every_cpu_no_pod_may_use_is_reserved_for_the_vendor_and_the_host():
    # Pods: the device layer's 8-9 and 10-15, the others' 16-19.
    assert shell("reserved") == "0-7"
    assert shell('ranges "3,4,5,8,9"') == "3-5,8-9"


def test_the_node_label_says_what_an_operator_or_the_tuner_must_do():
    ok = "ok     governor of every CPU: powersave\nok     isolated CPUs (kernel command line): 3-5,8-9\n"
    assert node_tuner.state(ok) == ("ok", "")
    drift = ok + "DRIFT  governor of every CPU: performance powersave (profile: powersave)\n"
    assert node_tuner.state(drift) == ("drift", "governor of every CPU: performance powersave (profile: powersave)")
    # A rule that only a restart of the host makes hold decides the label, whatever else drifts.
    value, report = node_tuner.state(drift + "REBOOT isolated CPUs (kernel command line): 3-5 (profile: 3-5,8-9)\n")
    assert value == "reboot-required" and "isolated CPUs" in report and "governor" in report
    kubelet = "KUBELET  kubelet's CPU manager: static, started before its configuration (profile: static, started with its configuration)\n"
    assert node_tuner.state(ok + kubelet)[0] == "kubelet-restart-required"
    assert node_tuner.state(ok + kubelet + "REBOOT  containerd-robot's affinity: 0-19 (profile: 16-19)\n")[0] == "reboot-required"


def test_a_restart_that_did_not_happen_is_tried_again_and_one_that_did_not_help_is_not():
    # s1u-02-x86, 11-13: the restart step failed before it called the vendor's service, and was never tried again.
    record, why = node_tuner.restart("host", {}, "p1", "boot-a", 1000.0)
    assert record == dict(kind="host", profile="p1", attempts=1, was="boot-a", at=1000.0) and why is None
    assert node_tuner.restart("host", record, "p1", "boot-a", 1060.0) == (None, "its restart was asked for 60 s ago")
    second, _ = node_tuner.restart("host", dict(record, said="ROS_MASTER_URI: unbound variable"), "p1", "boot-a", 1400.0)
    assert second["attempts"] == 2 and second["at"] == 1400.0
    third, _ = node_tuner.restart("host", second, "p1", "boot-a", 1800.0)
    assert node_tuner.restart("host", third, "p1", "boot-a", 2200.0) == (
        None, "3 attempts did not restart it; the last said: ROS_MASTER_URI: unbound variable")
    assert node_tuner.restart("host", record, "p1", "boot-b", 1400.0)[1].startswith("it was restarted for this profile")
    assert node_tuner.restart("host", third, "p2", "boot-a", 2200.0)[0]["attempts"] == 1
    assert node_tuner.restart("kubelet", third, "p1", "4242", 2200.0)[0] == dict(
        kind="kubelet", profile="p1", attempts=1, was="4242", at=2200.0)


class Tuned:
    """A robot's host and Node as its tuner meets them: what node_profile.sh says, the Node's labels and annotations
    as the tuner last published them, the other robots, the clock, the boot."""

    def __init__(self, monkeypatch, tmp_path, mode="all", status="REBOOT isolated CPUs (kernel command line): none\n"):
        self.calls, self.others, self.now, self.boot, self.directory = [], [], 10_000.0, "boot-a", tmp_path
        self.labels, self.annotations = {"rlark.io/node-category": "robot", node_tuner.POWER_STATE: "ready"}, {}
        self.said = {("status",): status, ("facts",): "record body serial=XCSU21\nrecord body parameters=0f0f\n",
                     ("reboot",): "restarting the robot for its node profile\n"}
        for name, value in (("NODE_NAME", "s1u-03-x86"), ("S1U_NODE_REBOOT", "idle"), ("S1U_NODE_ROLLOUT", mode),
                            ("S1U_NODE_ROLLOUT_SECONDS", "600"), ("S1U_NODE_ROLLOUT_PARALLEL", "1"),
                            ("S1U_NODE_POWER_OFF", "unattended"),
                            ("S1U_IMAGE", "registry/rlark/s1u-robot@sha256:1")):
            monkeypatch.setenv(name, value)
        monkeypatch.setattr(node_tuner, "HOST", str(tmp_path))
        monkeypatch.setattr(node_tuner, "LOWER_PASS_SECONDS", 0)
        monkeypatch.setattr(node_tuner, "host", self.host)
        monkeypatch.setattr(node_tuner, "node", lambda: (dict(self.labels), dict(self.annotations)))
        monkeypatch.setattr(node_tuner, "robots", lambda: self.others)
        monkeypatch.setattr(node_tuner, "publish", self.publish)
        monkeypatch.setattr(node_tuner, "running", lambda kind: self.boot)
        monkeypatch.setattr(node_tuner.time, "time", lambda: self.now)

    def host(self, *arguments, quiet=False):
        self.calls.append(arguments)
        said = self.said.get(arguments, "")
        return 0, said.pop(0) if isinstance(said, list) else said

    def publish(self, value, report, labels={}, **annotations):
        self.labels.update(labels, **{node_tuner.STATE: value})
        self.annotations.update(annotations, **{node_tuner.REPORT: report})

    def follow(self, seconds=60.0):
        self.now += seconds
        self.calls.clear()
        return node_tuner.follow("p1", "")

    def lease(self):
        return json.loads((self.directory / "maintenance").read_text())

    def maintenance(self):
        said = json.loads(self.annotations[node_tuner.MAINTENANCE])
        assert self.labels[node_tuner.MAINTENANCE_STATE] == said.get("state", "none")
        return said.get("state", "none"), said.get("why", "")

    def restarted(self, status="ok     everything\n"):
        """The host booted again: /run is empty, the rules read anew."""
        self.boot, self.said[("status",)] = self.boot + "+", status
        (self.directory / "maintenance").unlink()


def test_a_robot_nobody_uses_is_lowered_for_the_restart_its_profile_needs_and_comes_back(tmp_path, monkeypatch):
    robot = Tuned(monkeypatch, tmp_path, mode="staged")
    # A robot that is up queues on its Node, and until it has its lease the tuner says nothing to the power service.
    assert robot.follow() is True and robot.maintenance() == ("queued", "queued")
    assert not (tmp_path / "maintenance").exists() and ("reboot",) not in robot.calls
    # Its lease: off for a restart of the robot. The motors are on, as every robot's are all the time: no reason to
    # wait, the power service lowers it, and the restart is asked for once it is down.
    robot.said[("up", "host")] = "its motors are powered\n"
    assert robot.follow(10) is False and robot.maintenance() == ("lowering", "its motors are powered")
    lease = robot.lease()
    assert (lease["kind"], lease["power"], lease["held"], lease["why"]) == (
        "host", "off", True, "restarting the robot for its node profile") and lease["until"] == robot.now + 600
    assert ("reboot",) not in robot.calls and node_tuner.RESTART not in robot.annotations
    assert robot.annotations[node_tuner.REPORT].startswith(
        "restarting the robot for its node profile: lowering (its motors are powered); isolated CPUs")
    robot.said[("up", "host")] = ""
    robot.follow()
    assert robot.calls[-1] == ("reboot",) and robot.maintenance() == ("acting", "restarting the robot for its node profile")
    record = json.loads(robot.annotations[node_tuner.RESTART])
    assert (record["attempts"], record["was"], record["said"]) == (1, "boot-a", "restarting the robot for its node profile\n")
    assert robot.labels["rlark.io/rollout-state"] == "acting" and robot.lease()["since"] == lease["since"]
    # Not restarted yet: the lease stays, nothing is asked twice within five minutes; then it is, three times in all.
    robot.follow()
    assert ("reboot",) not in robot.calls and robot.lease()["held"] and robot.maintenance()[0] == "acting"
    robot.follow(300)
    assert robot.calls[-1] == ("reboot",) and json.loads(robot.annotations[node_tuner.RESTART])["attempts"] == 2
    # Three attempts did not restart it: failed, the lease given up, the robot back at its desired state.
    robot.follow(300)
    robot.follow(300)
    assert robot.maintenance() == ("failed", "3 attempts did not restart it; the last said: restarting the robot for its node profile\n")
    assert robot.lease()["kind"] == "" and ("reboot",) not in robot.calls
    # It restarted after all: /run is empty, the profile holds, no maintenance; the tuner's word lets the power
    # service bring the robot back to its desired state.
    (tmp_path / "maintenance").write_text("{}")
    robot.restarted()
    robot.follow()
    assert robot.maintenance() == ("none", "") and robot.lease()["kind"] == "" and robot.calls[-1] != ("reboot",)
    # A restart that did not help is not repeated, and the robot is not lowered for it.
    robot.said[("status",)] = "REBOOT isolated CPUs (kernel command line): none\n"
    robot.follow()
    assert robot.maintenance() == ("failed", "it was restarted for this profile and the rules still do not hold")
    assert robot.lease()["kind"] == "" and ("up", "host") not in robot.calls and robot.labels["rlark.io/rollout-state"] == "failed"
    # Whether it was restarted for this profile is the record's to say: without the record (an operator took it off
    # the Node), the restart is this profile's first again, at once.
    robot.annotations.pop(node_tuner.RESTART)
    robot.follow()
    robot.follow(10)
    assert robot.calls[-1] == ("reboot",) and robot.maintenance()[0] == "acting"


def test_a_maintenance_waits_for_whoever_uses_the_robot_and_for_nothing_else(tmp_path, monkeypatch):
    robot = Tuned(monkeypatch, tmp_path)
    # A Station serves a Run: it waits, as long as that takes, and that is no attempt.
    robot.said[("marked",)] = "fleet-station: the Station serves Run 20261116_101500_hg_dagger\n"
    for _ in range(60):
        robot.follow()
    assert robot.maintenance() == ("waiting", "in use (fleet-station: the Station serves Run 20261116_101500_hg_dagger)")
    assert robot.lease()["kind"] == "" and node_tuner.RESTART not in robot.annotations
    # A session let onto the robot between the lease's asking and its holding keeps the robot.
    robot.said[("marked",)] = ["", "s1u-device: a session is being admitted to the device layer\n"]
    robot.follow()
    assert robot.maintenance()[0] == "waiting" and robot.lease()["kind"] == "" and ("reboot",) not in robot.calls
    # The emergency stop pressed, the hardware in error: nothing is begun.
    robot.said[("marked",)] = ""
    robot.labels[node_tuner.POWER_STATE] = "estop"
    robot.follow()
    assert robot.maintenance() == ("waiting", "the robot is in estop") and robot.lease()["held"] is False
    # Arms fall when their motors lose power: where the site has not declared that safe with nobody there, a robot
    # whose motors are on waits for a person to power it off; one whose motors are off (a new robot) does not.
    robot.labels[node_tuner.POWER_STATE] = "ready"
    monkeypatch.setenv("S1U_NODE_POWER_OFF", "attended")
    robot.follow()
    assert robot.maintenance()[0] == "waiting" and robot.maintenance()[1].startswith(
        "its motors are on and powering them off is a person's") and robot.lease()["kind"] == ""
    robot.labels[node_tuner.POWER_STATE] = "off"
    robot.follow()
    assert robot.calls[-1] == ("reboot",) and robot.lease()["held"]
    (tmp_path / "maintenance").write_text("{}")
    robot.annotations.pop(node_tuner.RESTART)
    monkeypatch.setenv("S1U_NODE_POWER_OFF", "unattended")
    # Held, whoever marks the robot since (a Run's Station whose session the power service refused).
    robot.labels[node_tuner.POWER_STATE] = "ready"
    robot.said[("up", "host")] = "astribot_driver runs\n"
    robot.follow()
    robot.said[("marked",)] = "fleet-station: the Station serves Run 20261116_111500_hg_dagger\n"
    robot.follow()
    assert robot.maintenance() == ("lowering", "astribot_driver runs") and robot.lease()["held"]
    # A robot that does not come down: failed, the lease given up (its power service brings it back), again in an hour.
    robot.follow(300)
    assert robot.maintenance() == ("failed", "the robot did not come down within 300 s: astribot_driver runs")
    assert robot.lease()["kind"] == "" and ("reboot",) not in robot.calls
    robot.said[("marked",)] = ""
    robot.follow(1800)
    assert robot.maintenance()[0] == "failed" and robot.lease()["kind"] == ""
    robot.follow(1800)
    assert robot.maintenance()[0] == "lowering" and robot.lease()["held"]
    # Restarts of its own switched off: the label says what an operator must do, nothing is lowered.
    monkeypatch.setenv("S1U_NODE_REBOOT", "never")
    robot.follow()
    assert robot.maintenance() == ("none", "") and robot.lease()["kind"] == ""
    assert robot.labels[node_tuner.STATE] == "reboot-required"


def test_kubelets_restart_stops_the_driver_only_and_a_persons_request_is_a_maintenance_too(tmp_path, monkeypatch):
    robot = Tuned(monkeypatch, tmp_path, status="KUBELET kubelet's CPU manager: static, started before its configuration\n")
    robot.follow()
    assert robot.lease()["power"] == "powered" and robot.calls[-1] == ("restart-kubelet",)
    assert json.loads(robot.annotations[node_tuner.RESTART])["kind"] == "kubelet"
    robot.boot, robot.said[("status",)] = "4242", "ok     everything\n"
    robot.follow()
    assert robot.maintenance() == ("none", "") and robot.lease()["kind"] == ""
    # A restart on request (label): refused while the profile does not hold; else lowered and restarted like any
    # other, once, and after the boot it says done.
    robot.labels[node_tuner.REQUEST] = "7"
    robot.said[("status",)] = "DRIFT  governor of every CPU: performance (profile: powersave)\n"
    robot.follow()
    assert robot.annotations[node_tuner.REPORT].startswith("restart request 7 refused, the node profile does not hold")
    assert robot.lease()["kind"] == ""
    robot.said[("status",)] = "ok     everything\n"
    robot.follow()
    assert robot.calls[-1] == ("reboot",) and robot.lease()["why"] == "restarting the robot on request 7"
    asked = json.loads(robot.annotations[node_tuner.REQUESTED])
    assert (asked["profile"], asked["attempts"], asked["was"]) == ("7", 1, "4242")
    robot.restarted()
    robot.follow()
    robot.follow()
    asked = json.loads(robot.annotations[node_tuner.REQUESTED])
    assert asked["done"].endswith("Z") and asked["attempts"] == 1 and ("reboot",) not in robot.calls
    assert robot.maintenance() == ("none", "")


def test_the_orins_payload_is_switched_under_the_tuners_lease(tmp_path, monkeypatch):
    robot = Tuned(monkeypatch, tmp_path, status="ok     everything\n")
    robot.labels[node_spec.ORIN] = "updating"
    robot.annotations[node_tuner.ORIN_ALL] = json.dumps(dict(payloads={
        "quest-bridge": dict(wanted="56ca", running="1f00"), "lights": dict(wanted="aa", running="aa")}))
    robot.said[("marked",)] = "s1u-device: a session is attached to the device layer\n"
    robot.follow()
    assert robot.maintenance()[0] == "waiting"
    assert (tmp_path / "rollout").read_text() == "waiting: in use (s1u-device: a session is attached to the device layer)\n"
    # Nobody on the robot: the lease lowers nothing and keeps sessions off while the Orin's container switches.
    robot.said[("marked",)] = ""
    robot.follow()
    lease = robot.lease()
    assert (lease["kind"], lease["power"], lease["held"], lease["why"]) == (
        "payload", "ready", True, "switching the Orin to quest-bridge 56ca")
    assert (tmp_path / "rollout").read_text() == "allowed\n" and robot.maintenance()[0] == "acting"
    assert ("up", "host") not in robot.calls and ("reboot",) not in robot.calls
    robot.annotations[node_tuner.ORIN_ALL] = json.dumps(dict(payloads={"quest-bridge": dict(wanted="56ca", running="56ca")}))
    robot.labels[node_spec.ORIN] = "ok"
    robot.follow()
    assert robot.maintenance() == ("none", "") and robot.lease()["kind"] == ""
    assert (tmp_path / "rollout").read_text().startswith("waiting: ") and robot.labels["rlark.io/rollout-state"] == "done"
    # A payload the Orin does not switch to within ten minutes: failed, the lease given up.
    robot.annotations[node_tuner.ORIN_ALL] = json.dumps(dict(payloads={"quest-bridge": dict(wanted="77", running="56ca")}))
    robot.follow()
    robot.follow(601)
    assert robot.maintenance() == ("failed", "the Orin did not switch within 600 s") and robot.lease()["kind"] == ""


def test_no_more_robots_are_in_maintenance_at_once_than_the_addon_allows(tmp_path, monkeypatch):
    def other(name, state=None, since=0.0, change="image", why=""):
        return dict(name=name, wave=None, ready=True, since=0, rollout={}, maintenance=dict(
            state=state, since=since, change=change, why=why) if state else {})

    place = node_tuner.place
    assert place("c", None, [], "1", "image") == "queued"  # on its Node first, for the others to see
    assert place("c", 100.0, [], "1", "image") == ""
    # In the order they queued, by name where two queued in the same second; who holds a lease has a place.
    assert place("c", 100.0, [other("a", "queued", 90.0)], "1", "image") == "1 of the site's robots at a time; before it: a"
    assert place("c", 100.0, [other("a", "queued", 110.0), other("d", "queued", 100.0), other("b", "waiting", 1.0)], "1", "image") == ""
    assert place("c", 100.0, [other("a", "lowering"), other("b", "queued", 120.0)], "1", "image").endswith("before it: a")
    assert place("c", 100.0, [other("a", "acting"), other("b", "queued", 120.0)], "2", "image") == ""
    # A share of the site's robots, at least one.
    crowd = [other("r%02d" % number) for number in range(26)]
    assert place("c", 100.0, crowd + [other("a", "acting"), other("b", "acting")], "10%", "image") == ""
    assert place("c", 100.0, crowd + [other("a", "acting"), other("b", "acting"), other("d", "lowering")], "10%", "image").startswith("3 of")
    assert place("c", 100.0, [other("a", "acting")], "10%", "image").startswith("1 of")
    # A robot whose maintenance failed with this image stops the others; with another image it does not.
    assert place("c", 100.0, [other("a", "failed", why="the robot did not come down within 300 s: astribot_driver runs")],
                 "5", "image") == "a failed its maintenance with this image (the robot did not come down within 300 s: astribot_driver runs)"
    assert place("c", 100.0, [other("a", "failed", change="earlier")], "5", "image") == ""
    # Through a pass: three robots owe the restart, one at a time.
    robot = Tuned(monkeypatch, tmp_path, mode="staged")
    robot.others = [other("s1u-01-x86", "acting", change="registry/rlark/s1u-robot@sha256:1")]
    robot.follow()
    robot.follow(10)
    assert robot.maintenance() == ("queued", "1 of the site's robots at a time; before it: s1u-01-x86")
    assert robot.lease()["kind"] == "" and ("reboot",) not in robot.calls
    # Its motors are off while it waits (a new robot): it is not brought up meanwhile, only to come down again.
    robot.labels[node_tuner.POWER_STATE] = "off"
    robot.follow()
    lease = robot.lease()
    assert (lease["kind"], lease["held"]) == ("host", False) and lease["why"].startswith(
        "restarting the robot for its node profile: 1 of the site's robots at a time")
    # One that is up stays up and takes sessions while it waits.
    robot.labels[node_tuner.POWER_STATE] = "ready"
    robot.follow()
    assert robot.lease()["kind"] == ""
    robot.others = [other("s1u-01-x86")]
    robot.follow()
    assert robot.calls[-1] == ("reboot",) and robot.maintenance()[0] == "acting"


def test_who_uses_the_robot_is_what_the_components_mark_and_powered_motors_are_no_use(tmp_path):
    script = (TUNER / "node_profile.sh").read_text()
    library = script.rsplit('case "${1:-}" in', 1)[0].replace(
        "IN_USE=/run/rlark/in-use", "IN_USE=%s/in-use" % tmp_path).replace(
        "PROFILE=/etc/rlark/s1u/node_profile.env", "PROFILE=%s" % (TUNER / "node_profile.env"))
    (tmp_path / "library.sh").write_text(library)

    def said(command, driver="false"):
        return subprocess.run(["bash", "-c", "source %s; driver_runs() { %s; }; %s" % (
            tmp_path / "library.sh", driver, command)], capture_output=True, text=True).stdout.strip()

    assert said("marked") == ""  # no directory yet: whoever marks makes it
    (tmp_path / "in-use").mkdir()
    assert said("marked", driver="true") == ""  # the driver runs, as on every robot kept ready: nobody uses it
    (tmp_path / "in-use/fleet-station").write_text("the Station serves Run 20261116_101500_hg_dagger\n")
    (tmp_path / "in-use/s1u-device").write_text("a session is attached to the device layer\n")
    assert said("marked") == ("fleet-station: the Station serves Run 20261116_101500_hg_dagger\n"
                              "s1u-device: a session is attached to the device layer")
    # What a restart itself needs is another question: the driver stopped (and, for the host's, the motors off).
    assert said("up kubelet", driver="true") == "astribot_driver runs" and said("up kubelet") == ""


def test_a_node_conforms_when_every_item_of_the_specification_holds_and_like_nodes_have_one_fingerprint():
    text = ("model os=ubuntu 20.04\nmodel kernel=5.15.0-139-generic\nmodel container runtime version=1.7.24\n"
            "model registry credentials on the node=none\nmodel kubelet arguments=--config=/var/lib/kubelet/config.yaml\n"
            "site name servers=192.168.31.1\nbody grippers=left v1 right v1\nrecord body serial=XC%s\nrecord firmware=B%s\n")
    labels = {"rlark.io/robot-model": "s1u", "rlark.io/robot-id": "s1u-02", "rlark.io/node-category": "robot",
              "rlark.io/robot-body": "s1u", node_spec.ORIN: "ok", node_spec.CLOCK: "no_source"}
    failing, marks = node_spec.conformance("s1u-02-x86", node_spec.parse(text % (2, 2)), labels, {}, "ok", "")
    assert failing == ["%s (%s): not declared on the Node" % reference[:2] for reference in node_spec.REFERENCES]
    declared = {"rlark.io/node-install-spec": "v1.33.13-abc.0123456789ab", "rlark.io/node-install-state": "ok",
                "station.rlark.io/network-spec": "c0ffee", "station.rlark.io/network-state": "no-handshake",
                "rlark.io/node-labels-spec": "aa", "rlark.io/node-labels-state": "ok",
                "rlark.io/registry-trust.spec": "ab12", "rlark.io/registry-trust.state": "deferred ab12: config_path"}
    check = lambda: node_spec.conformance("s1u-02-x86", node_spec.parse(text % (2, 2)), labels, declared, "ok", "")
    assert check()[0] == ["site tunnel (station): no-handshake", "registry trust (registry-trust): deferred ab12: config_path"]
    declared.update({"station.rlark.io/network-state": "ok", "rlark.io/registry-trust.state": "applied ab12"})
    conforming, trusted = check()
    assert conforming == [] and trusted[node_spec.FINGERPRINT + "model"] != marks[node_spec.FINGERPRINT + "model"]
    other = dict(labels, **{"rlark.io/robot-id": "s1u-01", "rlark.io/robot-body": "s1u_observer"})
    _, others = node_spec.conformance("s1u-01-x86", node_spec.parse(text % (1, 1)), other, {}, "ok", "")
    model, site, body = (node_spec.FINGERPRINT + scope for scope in ("model", "site", "body"))
    assert marks[model] == others[model] and marks[site] == others[site] and marks[body] != others[body]
    drift = text.replace("5.15.0-139", "5.15.0-140").replace("node=none", "node=present") % (2, 2)
    labels.update({"rlark.io/robot-id": "s1u-07", node_spec.ORIN: "unreachable"})
    del labels["rlark.io/robot-body"]
    failing, changed = node_spec.conformance("s1u-02-x86", node_spec.parse(drift), labels, {}, "reboot-required", "10.254.240.1")
    assert changed[model] != marks[model] and changed[site] == marks[site]
    assert failing[:4] == [
        "node profile (node-profile): reboot-required",
        "kernel (vendor): 5.15.0-140-generic, specified 5.15.0-139-generic",
        "registry credentials on the node (registry-trust): present, specified none",
        "label rlark.io/robot-body (admission): missing"]
    assert failing[-2:] == ["Orin (orin): unreachable", "clock (clock): no_source"]


def test_nothing_is_installed_on_a_node_that_is_not_a_robot(monkeypatch):
    calls = []
    monkeypatch.setenv("NODE_NAME", "alexfu")
    monkeypatch.setattr(node_tuner.shutil, "copy", lambda *paths: None)
    monkeypatch.setattr(node_tuner, "host", lambda *arguments, quiet=False: calls.append(arguments) or (0, ""))
    monkeypatch.setattr(node_tuner, "node", lambda: ({"rlark.io/cluster-id": "rlark-robot-s1u-01"}, {}))

    class Stop(Exception):
        pass

    def stop(seconds):
        raise Stop

    monkeypatch.setattr(node_tuner.time, "sleep", stop)
    with pytest.raises(Stop):
        node_tuner.main()
    assert calls == []  # the control plane of the robot cluster is a workstation: its host is not touched


def test_a_wave_acts_after_the_earlier_ones_are_done_with_the_image_and_says_whom_it_waits_for(tmp_path, monkeypatch):
    """What can take a robot off line reaches a declared few first: a wrong version would otherwise restart every
    idle robot in the same minute (the Pods themselves are replaced everywhere at once)."""
    image, now = "registry/rlark/s1u-robot@sha256:2", 10_000

    def robot(name, wave, state=None, since=0, ready=True, ready_since=0, change=image, why=""):
        return dict(name=name, wave=wave, ready=ready, since=ready_since,
                    rollout=dict(change=change, state=state, since=since, why=why) if state else {})

    def gate(wave, *others):
        return node_tuner.gate(wave, image, list(others), 600, now)

    done_, fresh = robot("a", "0", "done", since=9_000), robot("a", "0", "done", since=9_900)
    assert gate("0", robot("b", "0"), robot("c", None)) == ""
    assert gate(None) == "" and gate(None, robot("c", None)) == ""
    assert gate(None, done_) == "" and gate("1", done_, robot("c", None)) == ""
    assert gate(None, fresh) == "wave 0: a is not done with this image for 600 s yet (done)"
    assert gate(None, done_, robot("b", "0", ready=False)) == ""
    assert gate(None, robot("b", "0", ready=False)) == "no robot of an earlier wave is Ready and done with this image"
    assert gate(None, done_, robot("b", "0", "acting", ready=False, ready_since=9_000)) == (
        "wave 0: b failed with this image (not Ready since it began)")
    assert gate(None, done_, robot("b", "0", "failed", why="3 attempts did not restart it")) == (
        "wave 0: b failed with this image (3 attempts did not restart it)")

    # Through a pass: a robot of the last wave waits for wave 0, then acts; the Orin's switch waits with it.
    tuned = Tuned(monkeypatch, tmp_path, mode="staged")
    monkeypatch.setenv("S1U_IMAGE", image)
    tuned.now, tuned.others = now - 60, [robot("s1u-01-x86", "0", "acting")]
    for other in tuned.others:
        other["maintenance"] = {}
    tuned.follow()
    assert ("reboot",) not in tuned.calls and tuned.labels["rlark.io/rollout-state"] == "waiting"
    assert tuned.maintenance()[1].startswith("wave 0: s1u-01-x86 is not done with this image")
    assert tuned.annotations[node_tuner.REPORT].startswith(
        "restarting the robot for its node profile: waiting (wave 0: s1u-01-x86 is not done")
    assert json.loads(tuned.annotations["rlark.io/rollout"])["why"].startswith("wave 0: s1u-01-x86")
    tuned.others[0].update(robot("s1u-01-x86", "0", "done", since=9_000), maintenance={})
    tuned.follow(0)
    tuned.follow(0)
    assert tuned.calls[-1] == ("reboot",) and tuned.labels["rlark.io/rollout-state"] == "acting"
    monkeypatch.setenv("S1U_NODE_ROLLOUT", "paused")
    (tmp_path / "maintenance").write_text("{}")
    tuned.follow(0)
    assert ("reboot",) not in tuned.calls and tuned.labels["rlark.io/rollout-state"] == "paused"
    assert tuned.maintenance() == ("waiting", "the Addon's parameter rollout is paused")
    # Done: the profile holds and the Orin is as the Addon declares.
    tuned.said[("status",)] = "ok     everything\n"
    tuned.labels[node_spec.ORIN] = "ok"
    tuned.follow(0)
    assert tuned.labels["rlark.io/rollout-state"] == "done"
    assert json.loads(tuned.annotations["rlark.io/rollout"]) == dict(change=image, state="done", why="", since=now)


def test_a_host_without_frequency_scaling_gets_the_rest_of_what_apply_sets(tmp_path):
    """Staging, 11-16: on a machine without cpufreq apply ended at the governor, every pass, before the default route
    and the scheduler's tunables."""
    script = (TUNER / "node_profile.sh").read_text()
    governors = script.split("apply() {", 1)[1].split("  sysctl -q -w", 1)[0].replace(
        "/sys/devices/system/cpu", str(tmp_path)).replace('source "$PROFILE"', "")
    run = "set -Eeuo pipefail; S1U_CPU_GOVERNOR=powersave\napply() {\n%s\n}\napply\necho reached" % governors
    assert subprocess.run(["bash", "-c", run], capture_output=True, text=True).stdout == "reached\n"
    (tmp_path / "cpu0/cpufreq").mkdir(parents=True)
    (tmp_path / "cpu0/cpufreq/scaling_governor").write_text("performance\n")
    assert subprocess.run(["bash", "-c", run], capture_output=True, text=True).stdout == "reached\n"
    assert (tmp_path / "cpu0/cpufreq/scaling_governor").read_text() == "powersave\n"


def test_the_x86_holds_its_own_address_only_and_everything_else_leaves_through_the_orin(tmp_path):
    """s1u_02 11-13: its Orin was leased the address of the site's network the x86 held by hand, and the x86 was gone.
    The x86 holds its own address, the Orin forwards under whatever address it has."""
    script = (TUNER / "node_profile.sh").read_text().replace(
        "/etc/rlark/s1u/node_profile.env", str(TUNER / "node_profile.env")).replace(
        "STATE=/run/rlark/s1u", "STATE=%s" % tmp_path)
    (tmp_path / "library.sh").write_text(script.rsplit('case "${1:-}" in', 1)[0])
    (tmp_path / "ip").write_text("""#!/bin/bash
case "$*" in
  "-4 -o addr show to 192.168.0.10") echo "2: enp2s0    inet 192.168.0.10/24 brd 192.168.0.255 scope global enp2s0" ;;
  "-4 -o addr show dev enp2s0") echo "2: enp2s0    inet 192.168.0.10/24 brd 192.168.0.255 scope global enp2s0"
     [[ ! -f $STUB/second ]] || echo "2: enp2s0    inet 192.168.31.250/32 scope global enp2s0" ;;
  "-4 route show default"*) [[ ! -f $STUB/routed ]] || echo "default via 192.168.0.11 dev enp2s0 proto static metric 100" ;;
  "route replace default via 192.168.0.11 dev enp2s0 proto static metric 100") touch $STUB/routed ;;
  "addr del 192.168.31.250/32 dev enp2s0") rm $STUB/second; echo "ip $*" >> $STUB/calls ;;
  "route del "*) echo "ip $*" >> $STUB/calls ;;
esac
""")
    (tmp_path / "nmcli").write_text("""#!/bin/bash
case "$*" in
  "-g GENERAL.CONNECTION device show enp2s0") echo "Wired connection 3" ;;
  "-g ipv4.addresses,ipv4.routes connection show Wired connection 3")
     if [[ -f $STUB/stale ]]; then printf '192.168.0.10/24, 192.168.31.250/32\n%s\n' "$(cat $STUB/stale)"; else echo 192.168.0.10/24; echo; fi ;;
  "-g ipv4.routes connection show Wired connection 3") cat $STUB/stale ;;
  "connection modify Wired connection 3 ipv4.addresses 192.168.0.10/24 ipv4.routes ") rm $STUB/stale; echo "nmcli $*" >> $STUB/calls ;;
  *) echo "nmcli $*" >> $STUB/calls ;;
esac
""")
    (tmp_path / "ping").write_text("#!/bin/bash\n[[ -f $STUB/masquerade && $* == *' registry.rlark.internal' ]]\n")
    (tmp_path / "hostname").write_text("#!/bin/sh\necho 192.168.0.10 192.168.31.251 10.254.240.7\n")
    for name in ("ip", "nmcli", "ping", "hostname"):
        (tmp_path / name).chmod(0o755)
    environment = {"PATH": "%s:/usr/bin:/bin" % tmp_path, "STUB": str(tmp_path)}
    rules = "#" + script.split("  # The uplink: this host holds its address", 1)[1].split('  rule "fair scheduler', 1)[0]

    def drift(addresses):
        (tmp_path / "orin_addresses").write_text(addresses + "\n")
        return subprocess.run(["bash", "-c", 'source %s; source $PROFILE; rule() { [[ $2 == "$3" ]] || echo "$1: $2"; }\ncheck() {\n%s\n}\ncheck'
                               % (tmp_path / "library.sh", rules)], capture_output=True, text=True, env=environment).stdout.splitlines()

    assert drift("192.168.31.5") == ["default route: ", "the site's registry answers this host (registry.rlark.internal): no"]
    default = "#" + script.split("  # Everything but the robot's own network leaves through the Orin", 1)[1].split("  local unit weight", 1)[0]
    for _ in range(2):
        subprocess.run(["bash", "-c", "source %s; source $PROFILE\ndefault() {\n%s\n}\ndefault" % (tmp_path / "library.sh", default)],
                       check=True, env=environment)
    assert (tmp_path / "calls").read_text() == "nmcli connection modify Wired connection 3 ipv4.gateway 192.168.0.11\n"
    (tmp_path / "masquerade").touch()
    assert drift("192.168.31.5") == []
    # What an earlier site left: an address and routes from it in the connection, and on the running host. Reported,
    # and taken off both by apply, RLark's own routes on the interface left alone; a second pass changes nothing.
    (tmp_path / "second").touch()
    (tmp_path / "stale").write_text("198.19.0.0/24 192.168.0.11 0 src=192.168.31.250, 10.11.0.0/16 192.168.0.11\n")
    assert drift("192.168.31.5") == [
        "addresses on the robot's interface enp2s0: 192.168.0.10 192.168.31.250",
        "what the connection of enp2s0 keeps for the next boot (addresses, routes): 192.168.0.10/24, 192.168.31.250/32 "
        "198.19.0.0/24 192.168.0.11 0 src=192.168.31.250, 10.11.0.0/16 192.168.0.11"]
    (tmp_path / "calls").unlink()
    said = [subprocess.run(["bash", "-c", "source %s; source $PROFILE\ndefault() {\n%s\n}\ndefault" % (tmp_path / "library.sh", default)],
                           check=True, env=environment, capture_output=True, text=True).stdout for _ in range(2)]
    assert (tmp_path / "calls").read_text().splitlines() == [
        "ip route del 198.19.0.0/24 via 192.168.0.11 dev enp2s0 proto static",
        "ip route del 10.11.0.0/16 via 192.168.0.11 dev enp2s0 proto static",
        "nmcli connection modify Wired connection 3 ipv4.addresses 192.168.0.10/24 ipv4.routes ",
        "ip addr del 192.168.31.250/32 dev enp2s0"]
    assert said[0].splitlines()[-1] == "address 192.168.31.250/32 taken off enp2s0" and said[1] == ""
    assert drift("192.168.31.5") == []
    # Room on the disk: each filesystem of kubelet's state, said before kubelet acts on it.
    (tmp_path / "df").write_text("""#!/bin/bash
[[ "$*" == "--output=target,pcent /var/lib /var/lib/kubelet" ]] || exit 2
printf 'Mounted on Use%%\\n/ %s%%\\n/ %s%%\\n' "$(cat $STUB/used)" "$(cat $STUB/used)"
""")
    (tmp_path / "df").chmod(0o755)
    space = "#" + script.split("  # Room on the disk, before kubelet removes images", 1)[1].split('  echo "note   CPUs', 1)[0]

    def room(used):
        (tmp_path / "used").write_text(str(used))
        return subprocess.run(["bash", "-c", "source %s; source $PROFILE\ncheck() {\n%s\n}\ncheck"
                               % (tmp_path / "library.sh", space)], capture_output=True, text=True, env=environment).stdout.splitlines()

    assert room(60) == ["ok     free space on /: 40 %"]
    assert room(81) == ["DRIFT  free space on /: 19 % (profile: at least 20 %)"]
    assert drift("192.168.31.251") == ["addresses of the Orin's uplink that this host holds too: 192.168.31.251"]
    assert drift("192.168.0.57 10.9.9.9") == [
        "addresses of the Orin's uplink inside the robot's own network 192.168.0.0/24: 192.168.0.57"]


def test_a_registry_credential_is_a_value_not_the_empty_section_containerd_keeps_for_it(tmp_path):
    """11-14: both robots' containerd configuration has an empty [plugins."io.containerd.grpc.v1.cri".registry.auths]
    header; the node specification read it as a credential and both nodes did not conform."""
    script = (TUNER / "node_profile.sh").read_text()
    pattern = re.search(r"grep -qiE '([^']+)' \"\$config\"", script.split("model registry credentials on the node=", 1)[1])[1]
    config = tmp_path / "config.toml"

    def read(text):
        config.write_text(text)
        return "present" if subprocess.run(["grep", "-qiE", pattern, str(config)]).returncode == 0 else "none"

    assert read('[plugins."io.containerd.grpc.v1.cri".registry]\n  [plugins."io.containerd.grpc.v1.cri".registry.auths]\n') == "none"
    assert read('[plugins."io.containerd.grpc.v1.cri".registry.configs."r:5443".auth]\n  username = "u"\n') == "present"


def test_the_clock_servo_closes_a_second_within_its_rate_limit_and_only_while_the_site_answers():
    import struct

    assert clock.ctypes.sizeof(clock.Timex) == 208  # struct timex on x86-64

    def stamp(seconds):
        return int(seconds) + clock.NTP_EPOCH, int(seconds % 1 * 2 ** 32)
    sent = 1000.0
    reply = struct.pack("!12I", *[0] * 8, *stamp(sent + 0.010 + 0.25), *stamp(sent + 0.012 + 0.25))
    offset, round_trip = clock.offset_of(reply, sent, sent + 0.022)
    assert abs(offset + 0.25) < 1e-6 and abs(round_trip - 0.020) < 1e-6
    servo, offset, history = clock.Servo(), -1.0, []
    for _ in range(600):
        ppm = servo.frequency(offset)
        assert abs(ppm) <= clock.LIMIT_PPM
        offset += ppm * 1e-6 * clock.PERIOD_SECONDS
        history.append(offset)
    locked = next(index for index, value in enumerate(history) if abs(value) <= clock.LOCKED_SECONDS)
    assert 25 * 60 < locked * clock.PERIOD_SECONDS < 40 * 60 and max(history) < clock.LOCKED_SECONDS
    assert abs(history[-1]) < 1e-4
    assert clock.hardware_clock([["bash"], ["sudo", "-S", "phc2sys", "-s", "nonexistent0", "-c", "CLOCK_REALTIME"]]) is None


def make_orin(tmp_path, monkeypatch, payloads=(), retired=()):
    """An Orin keeper with the payload directories given ({name: {file: text}}), its login in tmp_path."""
    monkeypatch.setattr(orin.tempfile, "mkdtemp", lambda dir: str(tmp_path))
    monkeypatch.setattr(orin, "ACCESS", str(tmp_path))
    monkeypatch.setattr(orin, "HOST", str(tmp_path))
    monkeypatch.setattr(orin, "CAMERAS", str(tmp_path))
    monkeypatch.setattr(orin, "IN_USE", tmp_path / "in-use")
    monkeypatch.setattr(orin, "PAYLOADS", tmp_path / "payloads")
    for name, files in dict(payloads).items():
        for file, text in files.items():
            (tmp_path / "payloads" / name / file).parent.mkdir(parents=True, exist_ok=True)
            (tmp_path / "payloads" / name / file).write_text(text)
    return orin.Orin("192.168.0.11", "astribot", "192.168.0.10", "s1u-02-x86", "led_driver/led1[.]py",
                     "Documents/system_setup.sh", list(retired))


def test_a_payload_has_one_version_of_its_files_and_a_changed_host_key_stops_everything_until_confirmed(tmp_path, monkeypatch):
    robot = make_orin(tmp_path, monkeypatch, {"quest-bridge": {"install": "#!/bin/sh\n", "bridge/bridge.py": "x"}})
    version, archive = robot.payloads["quest-bridge"]
    names = {member.name: member for member in tarfile.open(fileobj=io.BytesIO(archive))}
    assert set(names) == {"install", "bridge/bridge.py", "rlark.env", "VERSION"} and names["install"].mode == 0o755
    env = tarfile.open(fileobj=io.BytesIO(archive)).extractfile("rlark.env").read().decode()
    assert "S1U_ROS_IP_ADDRESS=192.168.0.10\n" in env and "S1U_ORIN_USER=astribot\n" in env
    # The version is the payload's files', not the robot's addresses.
    assert orin.payload(tmp_path / "payloads/quest-bridge", {})[0] == version
    (tmp_path / "payloads/quest-bridge/bridge/bridge.py").write_text("y")
    assert orin.payload(tmp_path / "payloads/quest-bridge", {})[0] != version
    assert [orin.delay(failures) for failures in (0, 1, 2, 4, 9)] == [60, 120, 240, 900, 900]
    assert [orin.verdict(*keys) for keys in ((None, "a"), ("a", "a"), ("a", "b"))] == ["first", "trusted", "changed"]
    # An Orin presenting another host key than the trusted one: nothing is sent to it until a person confirms the
    # presented fingerprint on the Node.
    monkeypatch.setenv("S1U_NODE_WANTED", "installed")
    sent, trusted = [], []
    monkeypatch.setattr(robot, "scan", lambda: ("ssh-ed25519 NEW", "SHA256:new"))
    record = {"fingerprint": "SHA256:old", "at": "2026-11-13T00:00:00Z", "how": "first use"}
    monkeypatch.setattr(robot, "known", lambda: ("ssh-ed25519 OLD", record))
    monkeypatch.setattr(robot, "ssh", lambda *arguments, **options: sent.append(arguments) or done())
    monkeypatch.setattr(robot, "trust", lambda *arguments: trusted.append(arguments) or {"how": arguments[2]})
    monkeypatch.setattr(orin.subprocess, "run", lambda *a, **k: type("R", (), {"stdout": "256 SHA256:old host (ED25519)"})())
    assert robot.reconcile()["label"] == "no_access" and not sent  # the secret is not mounted
    (tmp_path / "password").write_text("p\n")
    result = robot.reconcile()
    assert result["label"] == "identity_changed" and not sent and not trusted and result["trust"] == record
    assert (result["trusted_fingerprint"], result["presented_fingerprint"]) == ("SHA256:old", "SHA256:new")
    robot.confirm = robot.rotated = "SHA256:other"
    assert robot.reconcile()["label"] == "identity_changed" and not trusted
    robot.confirm = "SHA256:new"
    monkeypatch.setattr(robot, "root", lambda key, result: [])
    # Waiting for a person is no failure: the next look is a minute later, not backed off.
    robot.state["failures"] = 0
    monkeypatch.setattr(robot, "place", lambda key, steps, result: "")
    assert robot.reconcile()["label"] == "ok" and trusted == [("ssh-ed25519 NEW", "SHA256:new", "confirmed")]
    # The key RLark gave the Orin at the robot's join is on the Node before the Orin shows it: no person is asked.
    robot.confirm, robot.rotated = None, "SHA256:new"
    assert robot.reconcile()["label"] == "ok" and trusted[1:] == [("ssh-ed25519 NEW", "SHA256:new", "rotated by RLark")]
    stopped = PROFILE["S1U_ORIN_STOPPED"].strip('"')
    script = "/usr/bin/python3 /home/astribot/astribot_workspace/astribot_device_driver/led_driver/led1.py /dev/ttyLED0"
    assert re.search(stopped, script) and not re.search(stopped, "bash -c pkill -e -f -- '%s' || true" % stopped)


def test_payloads_are_placed_switched_when_the_host_is_free_and_removed_when_no_longer_declared(tmp_path, monkeypatch):
    robot = make_orin(tmp_path, monkeypatch, {"quest-bridge": {"install": "#!/bin/sh\n"}})
    version = robot.payloads["quest-bridge"][0]
    (tmp_path / "rollout").write_text("allowed\n")
    on, sent = {"declared": "old-thing", "quest-bridge": ""}, []

    def ssh(key, remote, data=b"", password=False):
        sent.append(remote)
        if remote.startswith("echo declared="):
            return done("".join("%s=%s\n" % item for item in on.items()).encode())
        return done()

    monkeypatch.setattr(robot, "ssh", ssh)
    monkeypatch.setattr(robot, "sudo", lambda key, remote: sent.append("sudo " + remote) or done())
    # Before the node tuner holds the robot's maintenance lease for it (somebody uses the robot, its wave or its
    # place has not come): the new version goes on the Orin and is installed, not switched to.
    (tmp_path / "rollout").write_text("waiting: in use (fleet-station: the Station serves Run 20261116)\n")
    result, steps = {}, []
    waiting = robot.place("KEY", steps, result)
    assert waiting == "quest-bridge %s is on the Orin; it starts with the robot's maintenance" % version
    assert "sudo ~/rl/quest-bridge.%s/install" % version in sent and not [line for line in sent if "install restart" in line]
    # The payload no longer declared goes, by its own removal first; the list on the Orin is the Addon's.
    assert "sudo sh -c '[ ! -x ~astribot/rl/old-thing/install ] || ~astribot/rl/old-thing/install remove'" in sent
    assert sent[-1] == "mkdir -p ~/rl && echo quest-bridge > ~/rl/.rlark-payloads"
    # The tuner's word: switched to.
    (tmp_path / "rollout").write_text("allowed\n")
    sent.clear()
    on["declared"] = "quest-bridge"
    assert robot.place("KEY", [], result) == ""
    assert "sudo ~/rl/quest-bridge.%s/install restart" % version in sent and result["payloads"] == {
        "quest-bridge": dict(wanted=version, running=version)}
    (tmp_path / "rollout").write_text("waiting: wave 0\n")
    sent.clear()
    assert robot.place("KEY", [], result).endswith("it starts with the robot's maintenance")
    on["quest-bridge"] = version
    sent.clear()
    assert robot.place("KEY", [], result) == "" and not [line for line in sent if line.startswith("sudo")]


def test_the_uplink_is_one_file_its_setup_script_is_the_addons_and_a_retired_unit_restarts_it(tmp_path, monkeypatch):
    unit = (ORIN / "s1u-orin-uplink.service").read_text()
    setup = orin.uplink_setup()
    assert unit in setup and "s1u-orin-uplink %s" % hashlib.sha256(unit.encode()).hexdigest()[:16] in setup
    # The script a new robot's Orin gets from Station API is this one, byte for byte.
    assert (CATALOG / "bootstrap/orin-uplink.sh").read_text() == setup
    (tmp_path / "systemctl").write_text("#!/bin/sh\necho \"$*\" >> %s/systemctl.calls\n[ \"$1\" != is-active ] || echo active\n" % tmp_path)
    (tmp_path / "systemctl").chmod(0o755)
    placed = setup.replace("/etc/systemd/system/s1u-orin-uplink.service", str(tmp_path / "placed.service"))
    for _ in range(2):
        subprocess.run(["sh"], input=placed, text=True, check=True, env={"PATH": "%s:/usr/bin:/bin" % tmp_path})
    assert (tmp_path / "placed.service").read_bytes() == unit.encode()
    assert (tmp_path / "systemctl.calls").read_text().count("daemon-reload") == 1
    # On the Orin: placed where it is not, and a unit retired by the Addon is stopped and the uplink started again.
    robot = make_orin(tmp_path, monkeypatch, retired=["fleet-orin-uplink"])
    there = dict(unit=hashlib.sha256(unit.encode()).hexdigest(), active="active", forwarding="1",
                 addresses="192.168.31.5", retired="fleet-orin-uplink", camera="", found="")
    sent = []

    def ssh(key, remote, data=b"", password=False):
        sent.append(remote)
        if remote.startswith("echo unit="):
            return done("".join("%s=%s\n" % item for item in there.items()).encode())
        return done(b"1000\n" if remote == "id -u" else b"")

    monkeypatch.setattr(robot, "ssh", ssh)
    monkeypatch.setattr(robot, "sudo", lambda key, remote: sent.append("sudo " + remote) or done())
    robot.root("KEY", {})
    assert "sudo sh -c 'systemctl disable --now --quiet fleet-orin-uplink.service; rm -f /etc/systemd/system/fleet-orin-uplink.service; " \
           "sh ~astribot/rl/.s1u-orin-uplink.sh && systemctl restart s1u-orin-uplink.service'" in sent
    assert (tmp_path / "orin_addresses").read_text() == "192.168.31.5\n"
    camera = next(line for line in sent if line.startswith("sudo sh -c 'install -m 644 ~astribot/rl/.s1u-camera.rules"))
    assert camera.endswith("|| { rm -f /etc/udev/rules.d/70-s1u-camera.rules; exit 1; }'")  # retried while udev did not take it
    # The cameras of both vendors are the account's: Orbbec's (head, torso) and the wrists' RealSense.
    assert "--attr-match=idVendor=2bc5 && udevadm trigger --subsystem-match=usb --attr-match=idVendor=8086 " in camera
    # A unit not there yet is placed by its setup script, which starts it: no restart beyond that (its stop step would
    # drop the x86's forwarding for a moment).
    there.update(unit="", retired="")
    sent.clear()
    robot.root("KEY", {})
    assert "sudo sh -c 'sh ~astribot/rl/.s1u-orin-uplink.sh'" in sent
    # Nothing to retire, the unit as it is, the rule there: nothing as root.
    there.update(unit=hashlib.sha256(unit.encode()).hexdigest(), retired="",
                 camera='SUBSYSTEM=="usb", ATTR{idVendor}=="2bc5", OWNER="1000", MODE="0660" '
                        'SUBSYSTEM=="usb", ATTR{idVendor}=="8086", OWNER="1000", MODE="0660"')
    sent.clear()
    robot.root("KEY", {})
    assert not [line for line in sent if line.startswith("sudo")]
    # What is asked, run by a real shell on a machine without any of it: every answer under its own name.
    asked = subprocess.run(["sh", "-c", next(line for line in sent if line.startswith("echo unit="))],
                           capture_output=True, text=True).stdout.splitlines()
    assert [line.split("=")[0] for line in asked] == ["unit", "active", "forwarding", "addresses", "retired", "camera", "found"]


def test_removing_takes_back_what_the_orin_got_and_sends_nothing_to_one_never_trusted(tmp_path, monkeypatch):
    monkeypatch.setenv("S1U_NODE_WANTED", "removed")
    robot = make_orin(tmp_path, monkeypatch)
    (tmp_path / "password").write_text("p\n")
    sent, patched = [], []

    def ssh(key, remote, data=b"", password=False):
        sent.append(remote)
        return done(b"declared=quest-bridge\n" if remote.startswith("echo declared=") else b"")

    monkeypatch.setattr(robot, "scan", lambda: ("ssh-ed25519 KEY", "SHA256:key"))
    monkeypatch.setattr(robot, "ssh", ssh)
    monkeypatch.setattr(robot, "sudo", lambda key, remote: sent.append("sudo " + remote) or done())
    monkeypatch.setattr(robot, "api", lambda *arguments: patched.append(arguments))
    monkeypatch.setattr(robot, "known", lambda: (None, None))
    assert robot.reconcile()["label"] == "removed" and not sent and not patched  # never trusted: never touched
    # Without the login: removed where no host key was ever trusted, else it cannot say.
    (tmp_path / "password").unlink()
    assert robot.reconcile()["label"] == "removed" and not sent
    monkeypatch.setattr(robot, "known", lambda: ("ssh-ed25519 KEY", {"fingerprint": "SHA256:key"}))
    assert robot.reconcile()["label"] == "no_access" and not sent
    (tmp_path / "password").write_text("p\n")
    monkeypatch.setattr(robot, "known", lambda: ("ssh-ed25519 KEY", {"fingerprint": "SHA256:key"}))
    (tmp_path / "in-use").mkdir()
    (tmp_path / "in-use/fleet-station").write_text("attached\n")
    assert robot.reconcile()["label"] == "removing" and sent == ["true"]
    (tmp_path / "in-use/fleet-station").unlink()
    assert robot.reconcile()["label"] == "removed"
    assert "sudo sh -c '[ ! -x ~astribot/rl/quest-bridge/install ] || ~astribot/rl/quest-bridge/install remove'" in sent
    assert "s/^#s1u-stopped# //' ~/Documents/system_setup.sh" in sent[-1] and "/ s1u-robot$/d" in sent[-1]
    assert patched == [("PATCH", "/api/v1/nodes/s1u-02-x86",
                        {"metadata": {"annotations": {orin.HOST_KEY: None, orin.TRUST: None, orin.SERIAL: None}}})]


def test_the_orin_resets_a_camera_by_its_serial_once_per_request(tmp_path, monkeypatch):
    """s1u_01 11-13 22:40: the head camera's own stream wedged and no restart of the vendor's camera driver brought it
    back. The camera service asks through a file the two containers share; the reset is the Orin account's (the
    cameras' rule), of the one device whose serial the vendor's camera_info.json gives the camera."""
    robot = make_orin(tmp_path, monkeypatch)
    sent, answer = [], ["head_rgbd bus 002 dev 008\n", 0]
    monkeypatch.setattr(robot, "known", lambda: ("ssh-ed25519 KEY", {}))
    monkeypatch.setattr(robot, "ssh", lambda key, remote, *a, **k: sent.append(remote) or done(
        answer[0].encode(), answer[1], b"PermissionError: [Errno 13]\n"))
    robot.camera_pass()
    assert not sent  # no request
    (tmp_path / "reset-request").write_text(json.dumps({"id": "a" * 32, "cameras": ["head_rgbd"]}))
    robot.camera_pass()
    assert sent[0].startswith("python3 -c '") and sent[0].endswith("' head_rgbd") and "sudo" not in sent[0]
    result = json.loads((tmp_path / "reset-result").read_text())
    assert set(result) == {"id", "ok", "detail", "reset", "missing", "finished_at"}  # the contract with the camera service
    assert result["id"] == "a" * 32 and result["ok"] and result["detail"] == "reset ok head_rgbd bus 002 dev 008"
    assert result["reset"] == ["head_rgbd"] and result["missing"] == []
    robot.camera_pass()
    assert len(sent) == 1  # the same request once
    # A restart of the container: the result beside the request says it was done.
    again = make_orin(tmp_path, monkeypatch)
    monkeypatch.setattr(again, "ssh", lambda *a, **k: sent.append("again") or done())
    again.camera_pass()
    assert len(sent) == 1 and again.camera["reset"]["id"] == "a" * 32
    # Two cameras asked for, one of them not on the bus: said, and not ok.
    (tmp_path / "reset-request").write_text(json.dumps({"id": "b" * 32, "cameras": ["head_rgbd", "left_wrist_rgbd"]}))
    robot.camera_pass()
    assert sent[-1].endswith("' head_rgbd left_wrist_rgbd")
    result = json.loads((tmp_path / "reset-result").read_text())
    assert not result["ok"] and result["detail"] == "reset ok head_rgbd bus 002 dev 008; no USB device of left_wrist_rgbd"
    assert result["reset"] == ["head_rgbd"] and result["missing"] == ["left_wrist_rgbd"]
    # A reset that did not run says neither: the camera service asks again later.
    (tmp_path / "reset-request").write_text(json.dumps({"id": "c" * 32, "cameras": ["head_rgbd"]}))
    answer[:] = ["", 1]
    robot.camera_pass()
    result = json.loads((tmp_path / "reset-result").read_text())
    assert result["detail"] == "PermissionError: [Errno 13]" and "reset" not in result and "missing" not in result
    # A reset that hangs (a wedged device) ends with ssh's timeout: done all the same, and not done again.
    (tmp_path / "reset-request").write_text(json.dumps({"id": "f" * 32, "cameras": ["head_rgbd"]}))

    def hung(*arguments, **keywords):
        sent.append("hung")
        raise subprocess.TimeoutExpired("ssh", 180)

    monkeypatch.setattr(robot, "ssh", hung)
    robot.camera_pass()
    robot.camera_pass()
    result = json.loads((tmp_path / "reset-result").read_text())
    assert sent.count("hung") == 1 and result["id"] == "f" * 32 and not result["ok"]
    assert result["detail"] == "TimeoutExpired: Command 'ssh' timed out after 180 seconds"
    # The script on an Orin: the device whose serial is the camera's, whatever its vendor, and no other.
    (tmp_path / "camera_info.json").write_text(json.dumps(
        {"head_rgbd": "CL8TC53000T", "torso_rgbd": "CP0HC5300034", "left_wrist_rgbd": "261123074156"}))
    script = orin.USB_RESET.replace("/sys/bus/usb/devices/*", str(tmp_path / "sys/*")).replace(
        "/dev/bus/usb/%03d/%03d", str(tmp_path / "dev/%03d-%03d")).replace("fcntl.ioctl(f, 0x5514)", "f.write(b'reset')").replace(
        "/etc/config/camera_info.json", str(tmp_path / "camera_info.json"))
    for name, serial, bus, number in (("2-1.3", "CL8TC53000T", 2, 8), ("2-3.2.6", "CP0HC5300034", 2, 9),
                                      ("2-3.4.4", "261123074156", 2, 10), ("1-1", None, 1, 3)):
        (tmp_path / "sys" / name).mkdir(parents=True)
        for key, value in (("serial", serial), ("busnum", bus), ("devnum", number)):
            if value is not None:
                (tmp_path / "sys" / name / key).write_text("%s\n" % value)
    (tmp_path / "dev").mkdir()
    said = subprocess.run([sys.executable, "-c", script, "head_rgbd", "left_wrist_rgbd", "right_wrist_rgbd"],
                          capture_output=True, text=True, check=True).stdout
    assert said == "head_rgbd bus 002 dev 008\nleft_wrist_rgbd bus 002 dev 010\n"
    assert (tmp_path / "dev/002-008").read_bytes() == b"reset" and (tmp_path / "dev/002-010").read_bytes() == b"reset"
    assert not (tmp_path / "dev/002-009").exists() and not (tmp_path / "dev/001-003").exists()
    assert "'" not in orin.USB_RESET  # it is sent inside single quotes


def test_the_orin_says_where_each_camera_is_and_what_it_shows_of_a_stuck_one(tmp_path, monkeypatch):
    """s1u-03 11-21: the head's Femto Bolt behind two hubs on the root port of the other cameras, its colour stream
    stopped while depth went on, the vendor's process running, no USB error. The facts are read as the Orin's account."""
    (tmp_path / "camera_info.json").write_text(json.dumps({"head_rgbd": "CL83C530054", "torso_rgbd": "CP0T2630001C",
                                                           "left_wrist_rgbd": "261123074156"}))
    sysfs = tmp_path / "sys"
    for name, keys in (("usb2", dict(serial="3610000.xhci", idVendor="1d6b", idProduct="0003")),
                       ("2-3", dict(idVendor="0bda", idProduct="0420")), ("2-3.3", dict(idVendor="0424", idProduct="5806")),
                       ("2-3.4", dict(idVendor="0424", idProduct="5806")),
                       ("2-3.3.3", dict(serial="CL83C530054", devnum="13", speed="5000", idVendor="2bc5")),
                       ("2-3.4.6", dict(serial="CP0T2630001C", devnum="10", speed="5000", idVendor="2bc5"))):
        (sysfs / name).mkdir(parents=True)
        for key, value in keys.items():
            (sysfs / name / key).write_text(value + "\n")
    for name, driver in (("2-3.3.3:1.0", "usbfs"), ("2-3.4.6:1.0", "uvcvideo")):
        (sysfs / name).mkdir()
        (tmp_path / "drivers" / driver).mkdir(parents=True)
        (sysfs / name / "driver").symlink_to(tmp_path / "drivers" / driver)
    stamp = lambda ago: time.strftime("[%m/%d %H:%M:%S.000001]", time.localtime(time.time() - ago))
    (tmp_path / "OrbbecSDK.log.txt").write_text("".join(line + "\n" for line in (
        "x" * 400000,
        "OB_SENSOR_COLOR Streaming... a line of a stack trace, without a stamp",
        stamp(60) + "[warning][1][Pipeline.cpp:377] Wait for frame timeout, you can try to increase the wait time!",
        stamp(40) + "[debug][3021309][VideoSensor.cpp:427] OB_SENSOR_COLOR Streaming... frameRate=30.2fps",
        stamp(39) + "[debug][3021311][VideoSensor.cpp:427] OB_SENSOR_DEPTH Streaming... frameRate=30.1fps",
        stamp(30) + "[warning][1][Pipeline.cpp:377] Wait for frame timeout, you can try to increase the wait time!",
        stamp(20) + "[warning][1][Pipeline.cpp:377] Wait for frame timeout, you can try to increase the wait time!",
        stamp(2) + "[debug][3021311][VideoSensor.cpp:427] OB_SENSOR_DEPTH Streaming... frameRate=30.1fps")))
    script = orin.CAMERA_FACTS.replace('"/sys/bus/usb/devices"', repr(str(sysfs))).replace(
        "/etc/config/camera_info.json", str(tmp_path / "camera_info.json")).replace("~/Log/OrbbecSDK.log.txt", str(tmp_path / "OrbbecSDK.log.txt"))
    facts = json.loads(subprocess.run([sys.executable, "-"], input=script, capture_output=True, text=True, check=True).stdout)
    assert facts["cameras"] == {
        "head_rgbd": dict(path="2-3.3.3", device=13, speed=5000, vendor="2bc5", hubs=["0bda:0420", "0424:5806"], driver="usbfs"),
        "torso_rgbd": dict(path="2-3.4.6", device=10, speed=5000, vendor="2bc5", hubs=["0bda:0420", "0424:5806"], driver="uvcvideo")}
    assert facts["sdk"]["color"] in (40, 41) and facts["sdk"]["depth"] in (2, 3) and facts["sdk"]["timeouts"] == 2  # after colour's last
    assert facts["process"] is False
    # Asked by the camera service: the facts once, a line for each camera, and where each one is for its report.
    robot = make_orin(tmp_path, monkeypatch)
    sent = []
    monkeypatch.setattr(robot, "ssh", lambda key, remote, data=b"", *a: sent.append((key, remote, data)) or done(
        json.dumps(dict(facts, process=True, sdk=dict(color=40, depth=2, timeouts=2))).encode()))
    robot.camera_pass()
    assert not sent  # no request, and nothing to log in with before the first pass
    (tmp_path / "diagnose-request").write_text(json.dumps({"id": "d" * 32, "cameras": ["head_rgbd", "torso_rgbd", "left_wrist_rgbd"]}))
    robot.camera_pass()
    assert json.loads((tmp_path / "diagnose-result").read_text())["cameras"] == dict.fromkeys(
        ["head_rgbd", "torso_rgbd", "left_wrist_rgbd"], "Orin 没有回答")
    robot.kept = dict(key="ssh-ed25519 KEY", trust=None, serial=None)
    (tmp_path / "diagnose-request").write_text(json.dumps({"id": "e" * 32, "cameras": ["head_rgbd", "torso_rgbd", "left_wrist_rgbd"]}))
    robot.camera_pass()
    assert sent == [("ssh-ed25519 KEY", "python3 -", orin.CAMERA_FACTS.encode())]
    result = json.loads((tmp_path / "diagnose-result").read_text())
    assert set(result) == {"id", "cameras", "finished_at"} and result["id"] == "e" * 32 and robot.camera["diagnose"] == result
    assert result["cameras"] == {
        "head_rgbd": "在 USB 2-3.3.3（5000M，经 2 级 hub）；厂商相机进程在跑；Orbbec SDK：彩色停了（40 秒前），深度仍在，之后等帧超时 2 次",
        "torso_rgbd": "在 USB 2-3.4.6（5000M，经 2 级 hub）；厂商相机进程在跑",  # uvcvideo's, not the SDK's
        "left_wrist_rgbd": "USB 上没有它（camera_info.json 的序列号）；厂商相机进程在跑"}
    assert json.loads((tmp_path / "usb").read_text()) == {
        "head_rgbd": dict(path="2-3.3.3", device=13, speed=5000, hubs=["0bda:0420", "0424:5806"]),
        "torso_rgbd": dict(path="2-3.4.6", device=10, speed=5000, hubs=["0bda:0420", "0424:5806"])}
    robot.camera_pass()
    assert len(sent) == 1  # the same request once
    # An Orin without the SDK's log: said so, not taken for both streams stopped.
    (tmp_path / "OrbbecSDK.log.txt").unlink()
    assert json.loads(subprocess.run([sys.executable, "-"], input=script, capture_output=True, text=True, check=True).stdout)["sdk"] is None
    monkeypatch.setattr(robot, "ssh", lambda *a, **k: done(json.dumps(dict(facts, process=True, sdk=None)).encode()))
    assert robot.diagnose_cameras(["head_rgbd"])["cameras"]["head_rgbd"] == "在 USB 2-3.3.3（5000M，经 2 级 hub）；厂商相机进程在跑；Orbbec SDK：没有日志"


def test_a_trusted_host_key_is_kept_on_the_robot_at_once(tmp_path, monkeypatch):
    robot = make_orin(tmp_path, monkeypatch)
    monkeypatch.setattr(orin, "KEPT", str(tmp_path / "orin_trust.json"))
    calls = []
    monkeypatch.setattr(robot, "api", lambda *arguments, **options: calls.append(arguments) or {})
    robot.serial = "17"
    record = robot.trust("ssh-ed25519 NEW", "SHA256:new", "confirmed")
    kept = json.loads((tmp_path / "orin_trust.json").read_text())
    assert kept == dict(key="ssh-ed25519 NEW", trust=record, serial="17") and record["how"] == "confirmed"
    assert calls[0][2]["metadata"]["annotations"][orin.CONFIRM] is None  # the confirmation is used up


def test_the_facts_need_no_time_source_variable(tmp_path):
    """11-14: Fleet's v0.11.7 tuner still restarted on a robot without a time source: its facts step (set -u) read the
    variable RLark does not render for an empty timeSource, printed nothing, and the tuner's KeyError followed."""
    script = (TUNER / "node_profile.sh").read_text()
    facts = script[script.index("facts() {"):script.index("\nweights() {")]
    line = next(line for line in facts.splitlines() if "site time source=" in line)
    said = subprocess.run(["bash", "-c", "set -u; %s" % line], capture_output=True, text=True,
                          env={"PATH": "/usr/bin:/bin"})
    assert said.returncode == 0 and said.stdout == "site time source=\n"


def test_the_tuner_tells_the_device_layer_which_robot_this_is_and_publishes_its_self_check(tmp_path, monkeypatch):
    monkeypatch.setattr(node_tuner, "HOST", str(tmp_path))
    labels = {"rlark.io/robot-id": "s1u_07", "rlark.io/robot-body": "s1u_observer"}
    # Not before the host's CPUs follow the profile, nor without the admission record's labels.
    node_tuner.identify(labels, "reboot-required")
    node_tuner.identify({"rlark.io/robot-id": "s1u_07"}, "ok")
    assert not (tmp_path / "robot.json").exists()
    node_tuner.identify(labels, "drift")
    assert json.loads((tmp_path / "robot.json").read_text()) == {"robot_identifier": "s1u_07", "body": "s1u_observer"}
    # The robot's name is its label's, whatever the node is called; a changed label is followed.
    node_tuner.identify(dict(labels, **{"rlark.io/robot-id": "s1u_08"}), "ok")
    assert json.loads((tmp_path / "robot.json").read_text())["robot_identifier"] == "s1u_08"
    # The self-check as the device layer leaves it: none yet, then each failed item by name.
    assert node_tuner.self_check() == ("pending", "{}")
    (tmp_path / "self-check.json").write_text(json.dumps(dict(
        state="failed", round="unpowered", failed=["相机 wrist_left"], checked_at=1794700000.0, items=[
            dict(name="ROS master", ok=True, detail="192.168.0.10:11311"),
            dict(name="相机 wrist_left", ok=False, detail="没有帧")])))
    value, annotation = node_tuner.self_check()
    assert value == "failed" and json.loads(annotation) == dict(
        round="unpowered", checked_at=1794700000.0, failed=[dict(name="相机 wrist_left", detail="没有帧")])
    # Details too long for the annotation are cut inside: it stays JSON within 2000 characters, every item named.
    (tmp_path / "self-check.json").write_text(json.dumps(dict(
        state="failed", round="powered", checked_at=1794700000.0, items=[
            dict(name="关节 %d" % joint, ok=False, detail='驱动报错 "E%d"\n' % joint * 100) for joint in range(5)])))
    value, annotation = node_tuner.self_check()
    assert len(annotation) <= 2000 and [item["name"] for item in json.loads(annotation)["failed"]] == [
        "关节 %d" % joint for joint in range(5)]


def test_the_host_boots_cgroup_v2_and_a_v1_host_is_told_to_restart_and_still_reports_the_rest(tmp_path):
    """11-17: kubelet 1.35 on refuses cgroup v1. Until the restart into v2, status reports every rule (the Pods' memory
    limit, the CPU weights and the vendor's threads, read from v2's tree, aside), apply leaves the CPU weights alone,
    and once on v2 kubelet starts without the installer's cgroup v1 allowance."""
    script = (TUNER / "node_profile.sh").read_text()
    grub = tmp_path / "grub"
    grub.write_text('GRUB_CMDLINE_LINUX_DEFAULT="quiet splash isolcpus=4-5 systemd.unified_cgroup_hierarchy=0"\n')
    line = script.split("  local line current wanted\n", 1)[1].split("  if [[ $current", 1)[0].replace("/etc/default/grub", str(grub))
    assert subprocess.run(["bash", "-c", "source %s/node_profile.env; isolated=$S1U_VENDOR_REALTIME_CPUS,$S1U_SDK_CPUS\n%secho \"$wanted\""
                           % (TUNER, line)], check=True, capture_output=True, text=True).stdout == \
        "quiet splash isolcpus=3-5,8-9 rcu_nocbs=8-9 systemd.unified_cgroup_hierarchy=1\n"
    script = script.replace("/etc/kubernetes/kubelet.conf.d", str(tmp_path)).replace("/var/lib/kubelet", str(tmp_path)) \
        .replace("/sys/fs/cgroup", str(tmp_path))
    library = tmp_path / "library.sh"
    library.write_text(script.rsplit('case "${1:-}" in', 1)[0])
    for name, text in {"stat": "#!/bin/sh\ncat $STUB/fs\n", "systemctl": "#!/bin/sh\necho \"systemctl $*\" >> $STUB/calls\n",
                       "df": "#!/bin/sh\nprintf 'Mounted on Use%%\\n/ 60%%\\n'\n"}.items():
        (tmp_path / name).write_text(text)
        (tmp_path / name).chmod(0o755)
    environment = {"PATH": "%s:/usr/bin:/bin" % tmp_path, "STUB": str(tmp_path)}
    rule, hierarchy = script.split("  cgroup=$(stat -fc %%T %s)\n" % tmp_path, 1)[1].split('  rule "isolated CPUs', 1)[0].split("  rule \"cgroup")
    hierarchy = rule + "  rule \"cgroup" + hierarchy
    weights = "  local unit weight\n" + script.split("  local unit weight\n", 1)[1].split("  # What Fleet left on the host", 1)[0]
    tree = "  # Read on cgroup v2 only: v1" + script.split("  # Read on cgroup v2 only: v1", 1)[1].split("  for unit in $(kubernetes_units)", 1)[0]
    memory = "  # Read on cgroup v2 only (v1" + script.split("  # Read on cgroup v2 only (v1", 1)[1].split('  echo "note   CPUs', 1)[0]

    def run(body):
        return subprocess.run(["bash", "-c", "set -Eeuo pipefail; source %s; source %s/node_profile.env; drift=0\n"
                               "check() {\ncgroup=$(stat -fc %%T .)\n%s\n%s\n}\ncheck" % (library, TUNER, rule, body)],
                              check=True, capture_output=True, text=True, env=environment).stdout.splitlines()

    (tmp_path / "fs").write_text("tmpfs\n")
    allowance = tmp_path / "90-rlark-cgroup-v1.conf"
    allowance.write_text("failCgroupV1: false\n")
    (tmp_path / "50-rlark-s1u.conf").write_text("cpuManagerPolicy: static\n")
    assert run(hierarchy) == ["REBOOT  cgroup hierarchy (kernel command line): tmpfs (profile: cgroup2fs)"]
    assert run(memory) == ["ok     free space on /: 40 %"]
    assert run(tree) == []
    assert run(weights) == [] and not (tmp_path / "calls").exists()
    run("kubelet")
    assert allowance.exists()
    (tmp_path / "fs").write_text("cgroup2fs\n")
    (tmp_path / "kubepods.slice").mkdir()
    (tmp_path / "kubepods.slice/memory.max").write_text(str(6144 * 1048576))
    assert run(hierarchy) == ["ok     cgroup hierarchy (kernel command line): cgroup2fs"]
    run(weights)
    assert "systemctl set-property --runtime system.slice CPUWeight=313" in (tmp_path / "calls").read_text().splitlines()
    assert run(memory) == ["ok     Pods' memory limit (MiB): 6144", "ok     free space on /: 40 %"]
    run("kubelet")
    assert not allowance.exists()


def test_an_unclaimed_idle_robot_whose_clock_is_off_has_its_driver_stopped_for_the_jump(tmp_path, monkeypatch):
    """C8: a clock more than 100 ms off the site's is jumped, and the clock container jumps only with the driver
    stopped: the tuner lowers a robot nobody holds or uses to powered for it, and lets it up again once it is."""
    robot = Tuned(monkeypatch, tmp_path, status="ok     everything\n")

    def follow(offset_ms, seconds=60.0):
        robot.now += seconds
        robot.calls.clear()
        (tmp_path / "clock.json").write_text(json.dumps(dict(offset_ms=offset_ms, at=robot.now - 5)))
        node_tuner.follow("p1", "10.254.240.1")

    (tmp_path / "claim").write_text(json.dumps(dict(holder="run_a")))
    follow(6082.6)
    assert robot.maintenance() == ("waiting", "Run run_a holds the robot") and robot.lease()["kind"] == ""
    (tmp_path / "claim").unlink()
    robot.said[("up", "clock")] = "astribot_driver runs\n"
    follow(6082.6)
    lease = robot.lease()
    assert (lease["kind"], lease["power"], lease["held"]) == ("clock", "powered", True)
    assert robot.maintenance() == ("lowering", "astribot_driver runs")
    robot.said[("up", "clock")] = ""
    follow(6082.6)
    assert robot.maintenance()[0] == "acting" and robot.lease()["held"]
    # Jumped: the lease is given up and the power service brings the driver back.
    follow(3.1)
    assert robot.maintenance() == ("none", "") and robot.lease()["kind"] == ""
    # A jump that does not come is given up, the driver let up again.
    follow(-250.0)
    follow(-250.0)
    follow(-250.0, seconds=node_tuner.CLOCK_SECONDS)
    assert robot.maintenance()[0] == "failed" and robot.lease()["kind"] == ""
