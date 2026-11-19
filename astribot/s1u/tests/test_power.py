"""The S1U's power service (astribot/s1u/device/power.py): the vendor page's rules, the robot in use, the vendor's
ROS on the host, the Unix socket."""
import contextlib
import http.client
import json
import socket
import sys
import threading
import time
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "device"))
import power  # noqa: E402

NONE = dict(kind="")  # the node tuner has spoken and no maintenance holds the robot


def test_the_state_and_the_vendor_pages_refusals():
    assert power.robot_state(None)["monitor"] is False
    on = power.robot_state(["12000000", "13000000"])
    assert on["powered"] and on["driver"] and on["ready"] and on["power_level"] == 0
    stop = power.robot_state(["12200100", "12000001"])
    assert stop["emergency_stop"] and stop["errors"] == ["12200100"] and not stop["driver"]
    assert power.robot_state(["12000000"], powered_off_s=3.0)["power_wait_s"] == 7.0
    assert power.refusal("power_on", power.robot_state(None)) == "vendor_monitor_down"
    assert power.refusal("power_on", power.robot_state(["12000000"], 3.0)) == "power_off_wait"
    assert power.refusal("power_off", on) == "driver_running" and power.refusal("power_off", on, force=True) is None
    assert power.refusal("driver_on", power.robot_state([])) == "not_powered"
    # In use (another component's mark): power off and restart wait, unless forced; the others do not.
    assert power.refusal("restart", on, busy="fleet-station: run") == "in_use"
    assert power.refusal("power_off", on, force=True, busy="fleet-station: run") is None
    assert power.refusal("driver_on", on, busy="fleet-station: run") is None


def test_the_codes_are_the_monitors_and_a_button_is_one_service_call():
    called = []
    service = power.Power(lambda name, value: called.append((name, value)) or (True, "ok"))
    service.receive("12000000,13000000")
    assert service.state()["ready"]
    answer = service.send("driver_off")
    assert answer == dict(sent="/astribot_system_monitor/driver_command", value=False, success=True, message="ok")
    assert called == [("/astribot_system_monitor/driver_command", False)]
    service.codes_at = time.monotonic() - 2  # stale: the monitor is down
    assert service.state()["monitor"] is False

    def failing(name, value):
        raise RuntimeError("service [/astribot_system_monitor/power_command] unavailable")

    service.call = failing
    service.receive("12000000")
    service.keep("off", "off")  # wanted off: the socket's own buttons may lower it
    status, body = service.act("power_off")
    assert status == 503 and "unavailable" in body["detail"]


def test_force_needs_a_reason_and_in_use_refuses_power_off(tmp_path, monkeypatch):
    monkeypatch.setattr(power, "IN_USE", tmp_path)
    service = power.Power(lambda name, value: (True, ""))
    service.receive("12000000")
    (tmp_path / "fleet-station").write_text("run 20261114\n")
    service.keep("off", "off")  # the desired state lets the robot be lowered
    assert service.act("power_off") == (409, dict(reason="in_use", state=service.state(), in_use="fleet-station: run 20261114"))
    assert service.act("power_off", force=True)[0] == 400  # force without a reason
    status, body = service.act("power_off", force=True, reason="robot must be moved")
    assert status == 200 and body["sent"].endswith("power_command") and service.powered_off_at is not None
    assert service.act("power_on")[1]["reason"] == "power_off_wait"


def test_shut_down_stops_the_driver_then_the_motors_and_waits_for_a_robot_in_use(tmp_path, monkeypatch):
    monkeypatch.setattr(power, "IN_USE", tmp_path)
    sent = []

    def call(name, value):
        sent.append((name, value))
        service.receive("12000000" if "driver_command" in name else "")
        return True, ""

    service = power.Power(call)
    service.keep("off", "off")
    service.receive("12000000,13000000")
    (tmp_path / "s1u-device").write_text("a session is attached to the device layer\n")
    status, body = service.act("shut_down")
    assert (status, body["reason"], body["in_use"]) == (409, "in_use", "s1u-device: a session is attached to the device layer")
    (tmp_path / "s1u-device").unlink()
    status, body = service.act("shut_down", sleep=lambda seconds: None)
    assert status == 200 and not body["state"]["powered"] and service.powered_off_at is not None
    assert sent == [("/astribot_system_monitor/driver_command", False), ("/astribot_system_monitor/power_command", False)]
    # Off already: nothing is sent.
    assert service.act("shut_down")[0] == 200 and len(sent) == 2


def test_bring_up_powers_waits_and_starts_the_driver(monkeypatch):
    codes = [["12000000"]]
    sent = []

    def call(name, value):
        sent.append((name, value))
        codes[0] = ["12000000", "13000000"] if "driver_command" in name else ["12000000"]
        return True, ""

    service = power.Power(call)
    service.receive("")  # the monitor publishes nothing powered

    def sleep(seconds):
        service.receive(",".join(codes[0]))

    monkeypatch.setattr(power.time, "monotonic", __import__("itertools").count(0.0, 0.1).__next__)  # 0.1 s a look
    status, body = service.act("bring_up", sleep=sleep)
    assert status == 200 and body["state"]["ready"]
    assert sent == [("/astribot_system_monitor/power_command", True), ("/astribot_system_monitor/driver_command", True)]


def test_the_service_answers_on_its_unix_socket_only(tmp_path):
    service = power.Power(lambda name, value: (True, ""))
    service.keep("powered", "powered")
    service.receive("12000000,13000000")
    path = tmp_path / "s1u-power.sock"
    server = power.Server(str(path), power.handler(service))
    threading.Thread(target=server.serve_forever, daemon=True).start()

    class Connection(http.client.HTTPConnection):
        def connect(self):
            self.sock = socket.socket(socket.AF_UNIX)
            self.sock.connect(str(path))

    def call(method, url, body=None):
        connection = Connection("localhost")
        connection.request(method, url, body=json.dumps(body) if body else None,
                           headers={"X-Remote-User": "operator-1", "Content-Type": "application/json"})
        response = connection.getresponse()
        return response.status, json.loads(response.read())

    assert call("GET", "/power")[1]["ready"]
    assert call("POST", "/power", {"action": "fly"})[0] == 400
    status, body = call("POST", "/power", {"action": "driver_off"})
    assert status == 200 and body["success"]
    server.shutdown()


class Robot:
    """The vendor's services as the robot answers them: the status codes follow each call, unless it is stuck."""

    def __init__(self, service=None):
        self.sent, self.stuck, self.service = [], False, service

    def __call__(self, name, value):
        action = (name.rsplit("/", 1)[1], value)
        self.sent.append(action)
        if not self.stuck:
            self.service.receive({("power_command", True): "12000000", ("driver_command", True): "12000000,13000000",
                                  ("driver_command", False): "12000000"}.get(action, ""))
        return True, ""


def robot(tmp_path, monkeypatch, codes=""):
    monkeypatch.setattr(power, "IN_USE", tmp_path / "in-use")
    (tmp_path / "in-use").mkdir(exist_ok=True)
    vendor = Robot()
    vendor.service = service = power.Power(vendor, tmp_path / "power.json")
    service.receive(codes)
    return service, vendor


def test_a_robot_wanted_ready_is_brought_up_and_brought_back_when_its_driver_dies(tmp_path, monkeypatch):
    service, vendor = robot(tmp_path, monkeypatch)
    # A robot that just joined has no desired state on its Node: wanted ready. Powered, then started, one step each.
    service.converge("", "", 0.0, NONE)
    service.converge("", "", 5.0, NONE)
    assert vendor.sent == [("power_command", True), ("driver_command", True)] and service.summary() == "ready"
    service.converge("", "", 10.0, NONE)
    assert len(vendor.sent) == 2 and service.recovered == [] and service.waiting == ""
    # Its driver dies (or someone stops it at the vendor's page): started again, and the recovery is recorded.
    service.receive("12000000")
    service.converge("ready", "", 15.0, NONE)
    assert vendor.sent[-1] == ("driver_command", True) and service.summary() == "ready"
    assert [(entry["action"], entry["was"]) for entry in service.recovered] == [("driver_on", "powered")]
    # A driver that does not come up is tried again after 30 s, then 60 s, not every few seconds.
    service.receive("12000000")
    vendor.stuck = True
    for now in (20.0, 25.0, 45.0, 51.0, 60.0, 110.0, 112.0):
        service.converge("ready", "", now, NONE)
    assert vendor.sent[3:] == [("driver_command", True)] * 2 and "did not take" in service.waiting  # at 45 s and 110 s


def test_a_robot_wanted_off_or_powered_is_left_alone_and_lowered_only_when_a_person_lowers_it(tmp_path, monkeypatch):
    service, vendor = robot(tmp_path, monkeypatch, "12000000,13000000")
    service.converge("ready", "", 0.0, NONE)
    # A person stops the driver at RLark: the desired state is powered. The driver is stopped, the motors stay on.
    service.converge("powered", "", 5.0, NONE)
    service.converge("powered", "", 10.0, NONE)
    assert vendor.sent == [("driver_command", False)] and service.summary() == "powered"
    # Powered off at RLark while a session is attached: it waits, and says so; then the motors go off.
    (tmp_path / "in-use" / "s1u-device").write_text("a session is attached to the device layer\n")
    service.converge("off", power.in_use(), 15.0, NONE)
    assert len(vendor.sent) == 1 and service.waiting.startswith("in use (s1u-device")
    (tmp_path / "in-use" / "s1u-device").unlink()
    service.converge("off", "", 20.0, NONE)
    service.converge("off", "", 25.0, NONE)
    assert vendor.sent[-1] == ("power_command", False) and service.summary() == "off"
    # Wanted off, it is left alone: nothing brings it up, through a restart of the service too, and someone who powers
    # it on at the vendor's page is not undone either.
    restarted = power.Power(vendor, tmp_path / "power.json")
    vendor.service = restarted
    restarted.receive("")
    restarted.converge("off", "", 30.0, NONE)
    restarted.receive("12000000,13000000")
    restarted.converge("off", "", 35.0, NONE)
    assert len(vendor.sent) == 2 and restarted.recovered == []
    # On the socket nobody lowers a robot below its desired state: that is lowered at RLark.
    assert restarted.act("driver_off")[0] == 200 and vendor.sent[-1] == ("driver_command", False)
    restarted.converge("ready", "", 60.0, NONE)
    refused = restarted.act("shut_down")
    assert (refused[0], refused[1]["reason"], refused[1]["desired"]) == (409, "desired", "ready")


def test_an_emergency_stop_or_a_hardware_error_is_a_named_state_and_nothing_is_pulled_up_under_it(tmp_path, monkeypatch):
    service, vendor = robot(tmp_path, monkeypatch, "12200100")
    for now in (0.0, 5.0, 60.0):
        service.converge("ready", "", now, NONE)
    assert vendor.sent == [] and service.summary() == "estop" and service.waiting == "emergency stop"
    service.receive("12200201")
    service.converge("ready", "", 65.0, NONE)
    assert vendor.sent == [] and service.summary() == "fault" and service.waiting == "errors 12200201"
    # Cleared, it converges.
    service.receive("12000000")
    service.converge("ready", "", 70.0, NONE)
    assert vendor.sent == [("driver_command", True)]


def test_a_robot_a_run_used_is_wanted_ready_from_then_on_until_it_is_set_again_at_rlark(tmp_path, monkeypatch):
    service, vendor = robot(tmp_path, monkeypatch)
    service.converge("off.1794729000", "", 0.0, NONE)
    assert vendor.sent == [] and service.desired == "off"
    # A Run uses it: the device layer brings it up for the session, whatever was desired.
    mark = tmp_path / "in-use" / "s1u-device"
    mark.write_text("a session is being admitted to the device layer\n")
    assert service.act("bring_up", sleep=lambda seconds: service.receive(",".join(service.codes)))[0] == 200
    service.converge("off.1794729000", power.in_use(), 5.0, NONE)
    assert service.summary() == "ready" and service.desired == "ready"
    # The Run ends: it stays wanted ready (nothing is lowered), and a driver that dies is brought back, through a
    # restart of the service too.
    mark.unlink()
    service.converge("off.1794729000", "", 10.0, NONE)
    assert vendor.sent == [("power_command", True), ("driver_command", True)]
    restarted = power.Power(vendor, tmp_path / "power.json")
    vendor.service = restarted
    restarted.receive("12000000")
    restarted.converge("off.1794729000", "", 15.0, NONE)
    assert vendor.sent[-1] == ("driver_command", True) and restarted.desired == "ready"
    # A person powers it off at RLark again (the same state, a new value): that is in force.
    for now in (20.0, 25.0, 30.0):
        restarted.converge("off.1794729900", "", now, NONE)
    assert vendor.sent[-2:] == [("driver_command", False), ("power_command", False)] and restarted.desired == "off"


def test_a_robot_whose_desired_state_was_never_read_is_left_as_it_is(tmp_path, monkeypatch):
    """A power service that starts while the API server is away, with nothing kept on the host, does not know what is
    wanted: it calls nothing, whatever the robot is, until the Node has been read."""
    service, vendor = robot(tmp_path, monkeypatch)
    for _ in range(3):
        service.converge(service.label, "", 0.0, NONE)
    assert service.label is None and vendor.sent == [] and "has not been read" in service.waiting
    assert not (tmp_path / "power.json").exists()
    # Read: wanted off, and the robot is off already.
    service.converge("off.1794737413", "", 5.0, NONE)
    assert vendor.sent == [] and service.desired == "off" and service.waiting == ""
    # A restart while the API server is away keeps what was read.
    restarted = power.Power(vendor, tmp_path / "power.json")
    restarted.converge(restarted.label, "", 0.0, NONE)
    assert restarted.label == "off.1794737413" and vendor.sent == []


def test_a_maintenance_lowers_the_robot_keeps_sessions_off_and_ends_at_the_desired_state(tmp_path, monkeypatch):
    monkeypatch.setattr(power, "IN_USE", tmp_path / "in-use")
    monkeypatch.setattr(power, "MAINTENANCE", tmp_path / "maintenance")
    sent = []

    def call(name, value):
        sent.append((name.rsplit("/", 1)[1], value))
        codes = set(service.codes)
        code = "13000000" if "driver" in name else "12000000"
        service.receive(",".join(sorted(codes | {code} if value else codes - {code})))
        return True, ""

    service = power.Power(call, tmp_path / "power.json")
    service.receive("")
    # A host that just booted: before the tuner's first word the robot is not brought up, a session may bring it up.
    assert power.maintenance() is None
    service.converge("", "", 0.0, power.maintenance())
    assert sent == [] and service.waiting.startswith("the node tuner has not said yet")
    lease = dict(kind="host", of="host p1", power="off", why="restarting the robot for its node profile", since=0)
    (tmp_path / "maintenance").write_text(json.dumps(dict(kind="", until=time.time() + 600)))
    for now in (5.0, 10.0):
        service.converge("", "", now, power.maintenance())
    assert sent == [("power_command", True), ("driver_command", True)] and service.summary() == "ready"
    # A robot that is off under a lease asked for and not held (its restart is due) is not brought up.
    off = power.Power(lambda name, value: sent.append(("unexpected", name)) or (True, ""))
    off.receive("")
    off.converge("", "", 0.0, dict(lease, held=False))
    assert off.summary() == "off" and off.waiting.startswith("maintenance: ")
    # Asked for and not held yet: nothing is lowered, and nobody new is let on.
    (tmp_path / "maintenance").write_text(json.dumps(dict(lease, held=False, until=time.time() + 600)))
    service.converge("", "", 15.0, power.maintenance())
    assert len(sent) == 2
    assert service.act("bring_up") == (409, dict(
        reason="maintenance: restarting the robot for its node profile", state=service.state()))
    assert service.act("driver_on")[1]["reason"].startswith("maintenance")
    # Held: down to what the maintenance needs whatever marks the robot (a Run's Station that was refused), the
    # desired state as it was, on the Node and on the host.
    (tmp_path / "maintenance").write_text(json.dumps(dict(lease, held=True, until=time.time() + 600)))
    (tmp_path / "in-use").mkdir()
    (tmp_path / "in-use/fleet-station").write_text("the Station serves Run 20261116\n")
    for now in (20.0, 25.0, 30.0, 35.0):
        service.converge("", power.in_use(), now, power.maintenance())
    assert sent[2:] == [("driver_command", False), ("power_command", False)] and service.summary() == "off"
    assert service.desired == "ready" and json.loads((tmp_path / "power.json").read_text())["desired"] == "ready"
    assert service.waiting == "maintenance: restarting the robot for its node profile"
    # The host restarted (no lease file, the kept state read again): up only after the tuner's word, and that is no
    # recovery.
    (tmp_path / "maintenance").unlink()
    (tmp_path / "in-use/fleet-station").unlink()
    restarted = power.Power(call, tmp_path / "power.json")
    restarted.receive("")
    service = restarted
    restarted.converge("", "", 0.0, power.maintenance())
    assert len(sent) == 4
    (tmp_path / "maintenance").write_text(json.dumps(dict(kind="", until=time.time() + 600)))
    for now in (5.0, 10.0, 15.0):
        restarted.converge("", "", now, power.maintenance())
    assert sent[4:] == [("power_command", True), ("driver_command", True)] and restarted.recovered == []
    # kubelet's restart stops the driver only; a lease its tuner no longer renews is over.
    kubelet = dict(kind="kubelet", of="kubelet p1", power="powered", why="restarting kubelet for its node profile", since=0)
    (tmp_path / "maintenance").write_text(json.dumps(dict(kubelet, held=True, until=time.time() + 600)))
    for now in (20.0, 25.0):
        restarted.converge("", "", now, power.maintenance())
    assert sent[6:] == [("driver_command", False)] and restarted.summary() == "powered"
    (tmp_path / "maintenance").write_text(json.dumps(dict(kubelet, held=True, until=time.time() - 1)))
    assert power.maintenance() == dict(kind="")
    for now in (30.0, 35.0):
        restarted.converge("", "", now, power.maintenance())
    assert sent[7:] == [("driver_command", True)] and restarted.summary() == "ready"
    # A robot a person powered off is not raised by a maintenance's end.
    for now in (40.0, 45.0, 50.0):
        restarted.converge("off.1794729000", "", now, power.maintenance())
    assert restarted.summary() == "off"
    count = len(sent)
    (tmp_path / "maintenance").write_text(json.dumps(dict(lease, held=True, until=time.time() + 600)))
    restarted.converge("off.1794729000", "", 55.0, power.maintenance())
    (tmp_path / "maintenance").write_text(json.dumps(dict(kind="", until=time.time() + 600)))
    for now in (60.0, 65.0):
        restarted.converge("off.1794729000", "", now, power.maintenance())
    assert len(sent) == count and restarted.summary() == "off" and restarted.desired == "off"


def test_the_motors_go_off_no_sooner_than_the_models_seconds_after_the_driver_was_last_seen(tmp_path, monkeypatch):
    """User 11-16: the driver first, under which the arms sink slowly, and the power five seconds later. Every way to
    off: the socket's button, shut_down, a lowered desired state, a maintenance."""
    monkeypatch.setattr(power, "IN_USE", tmp_path / "in-use")
    clock = [1000.0]
    monkeypatch.setattr(power.time, "monotonic", lambda: clock[0])
    sent = []

    def call(name, value):
        sent.append((name.rsplit("/", 1)[1], value, clock[0]))
        codes = set(service.codes)
        code = "13000000" if "driver" in name else "12000000"
        service.receive(",".join(sorted(codes | {code} if value else codes - {code})))
        return True, ""

    def tick(seconds):  # the monitor publishes on, the clock goes
        clock[0] += seconds
        service.receive(",".join(service.codes))

    service = power.Power(call, driver_stop_seconds=5.0)
    service.receive("12000000,13000000")
    # A lowered desired state: the driver, then nothing for five seconds, then the motors.
    service.converge("ready", "", clock[0], NONE)
    for _ in range(4):
        tick(1.5)
        service.converge("off", "", clock[0], NONE)
    assert [step[:2] for step in sent] == [("driver_command", False)] and service.waiting.startswith("power_off ")
    # Read in between, the robot is powered (what the Node's power-state says), never ready straight to off.
    assert service.state()["driver_stop_wait_s"] == 0.5 and service.summary() == "powered"
    tick(1.5)
    service.converge("off", "", clock[0], NONE)
    assert sent[-1][:2] == ("power_command", False) and sent[-1][2] - sent[0][2] >= 5.0
    # The socket's power-off button within the five seconds is refused, unless forced with a reason.
    service.receive("12000000,13000000")
    service.keep("off", "off")
    tick(1.0)
    service.receive("12000000")
    tick(2.0)
    assert service.act("power_off")[1]["reason"] == "driver_stop_wait"
    # A forced press overrides the vendor page's refusals, not the arms' time.
    assert service.act("power_off", force=True, reason="robot must be moved")[1]["reason"] == "driver_stop_wait"
    tick(3.0)  # five seconds since the driver's codes were last seen
    assert service.act("power_off")[0] == 200
    # shut_down waits the seconds out itself.
    service.receive("12000000,13000000")
    sent.clear()
    status, body = service.act("shut_down", sleep=tick)
    assert status == 200 and [step[:2] for step in sent] == [("driver_command", False), ("power_command", False)]
    assert sent[1][2] - sent[0][2] >= 5.0
    # A maintenance's lowering is no exception.
    monkeypatch.setattr(power, "MAINTENANCE", tmp_path / "maintenance")
    service.receive("12000000,13000000")
    service.keep("ready", "ready")
    sent.clear()
    lease = dict(kind="host", power="off", why="restarting the robot for its node profile", held=True)
    for _ in range(5):
        service.converge("ready", "fleet-station: a Run", clock[0], lease)
        tick(1.5)
    assert [step[:2] for step in sent] == [("driver_command", False), ("power_command", False)] and sent[1][2] - sent[0][2] >= 5.0


def test_the_claim_file_follows_the_watched_nodes_label_and_stays_while_the_api_server_is_away(tmp_path, monkeypatch):
    monkeypatch.setattr(power, "CLAIM_FILE", tmp_path / "claim")
    monkeypatch.setenv("NODE_NAME", "s1u-03-x86")
    seen, files, asked = {}, [], []

    def claim():
        return json.loads((tmp_path / "claim").read_text()) if (tmp_path / "claim").exists() else None

    def told(*events):
        """One watch as the API server streams it: each event, then the claim file as that event left it."""
        for kind, labels in events:
            yield json.dumps(dict(type=kind, object=dict(metadata=dict(labels=labels)))).encode() + b"\n"
            files.append(claim())

    # A watch that tells the Node, a watch error and a change; the API server away; a watch that tells no claim.
    watches = iter([told(("ADDED", {power.CLAIM: "run-a", power.WANTED: "off"}), ("ERROR", {}),
                         ("MODIFIED", {power.CLAIM: "run-b"})), OSError("away"), told(("ADDED", {}))])

    def node(query, patch=None, timeout=10):
        asked.append((query, timeout))
        reply = next(watches)  # the last watch is over: StopIteration ends watch()
        if isinstance(reply, OSError):
            raise reply
        return contextlib.nullcontext(reply)

    monkeypatch.setattr(power, "node", node)
    monkeypatch.setattr(power.time, "sleep", lambda _: files.append(claim()))
    with pytest.raises(StopIteration):
        power.watch(seen)
    assert files == [dict(holder="run-a"), dict(holder="run-a"), dict(holder="run-b"), dict(holder="run-b"), None]
    assert asked[0] == ("?watch=true&fieldSelector=metadata.name%3Ds1u-03-x86&timeoutSeconds=30", 35) and seen == dict(labels={})
