"""The S1U's power, driver start and restart without the vendor's page (user 11-12): the three buttons of Astribot's
control centre (电源 / 启动 / 重启) as a service of the Addon s1u-robot on every S1U robot x86, for RLark's API.

It does what the vendor's web server does behind those buttons, nothing else: one std_srvs/SetBool call to the
vendor's system monitor, which runs the vendor's own scripts.

    电源  /astribot_system_monitor/power_command   True: joint motors on, False: off
    启动  /astribot_system_monitor/driver_command  True: astribot_driver and its activation, False: stop it
    重启  /astribot_system_monitor/reboot_command  True: motors off, the Orin restarted, then this x86

The vendor's services answer at once, before anything has happened, and check nothing; the checks are in the vendor's
page, and so here (refusal): the driver starts only on powered motors, the motors go off under a running driver only
when forced, and come on again no sooner than 10 s after they went off. The way to off is the model's own (user 11-16):
the driver stopped first, under which the arms sink slowly, and the motors off no sooner than the node profile's
S1U_DRIVER_STOP_SECONDS after the driver was last seen running; every power-off here waits for that, a button's (409
driver_stop_wait, forced or not: force only overrides the vendor page's own refusals), shut_down's and the
convergence's. On the Node a robot lowered from ready to off reads powered while its arms sink. Power off and restart also wait while the robot
is in use (a component's mark under /run/rlark/in-use: a session attached to the device layer, a Station with a
Run), unless forced with a reason. The state is the vendor's: its system
monitor publishes every component's status codes (/astribot_error_code/multi_error_code, 30 Hz); a code's digits are
module (12 power and hardware, 13 driver), level (0 normal, 1 warning, 2 error) and part.

It speaks ROS itself (rospy, the vendor's noetic) to the robot's master at S1U_ROS_IP_ADDRESS, as a node at that address
(the container is on the host's network). It listens on a Unix socket only (S1U_SERVICE_SOCKET); RLark reaches it on
the node and decides who may press, and records who did.

    GET  /power   the state: monitor, powered, driver, emergency_stop, ready, errors, power_level, driver_level, power_wait_s,
                  driver_stop_wait_s, restart (the last restart asked for at RLark and its phase, or null), desired
                  (off | powered | ready, the desired state in force), waiting (why it is not converging now, "" when
                  nothing waits), station (the Node's label rlark.io/station, "S03", or null) and station_mark (the
                  Node's record rlark.io/station-mark-set, {mark: disabled | repair, reason, by, at}, or null), the
                  last two from the Node as last watched
    POST /power   {"action": power_on | power_off | driver_on | driver_off | restart | bring_up | shut_down,
                   "force": false, "reason": "..."}: 200 {sent, value, success, message, state}; 409 {reason, state}
                   when refused; 400 when force comes without a reason; 503 when the vendor's service failed; 504 when
                   bring_up did not end ready or shut_down did not end off
bring_up is from off to ready in one press (the motors, then the driver), shut_down the way back (the driver, then the
motors).

    POST /pose    {"pose": "home", "action": "hold" | "stop"}: the vendor page's hold-to-move 初始姿势 (Home Pose). The
                  first hold sends the vendor's web server (S1U_ROS_IP_ADDRESS:5141, POST /msg) pose_running, each
                  further hold keeps the robot moving, and pose_stop is sent on stop or once no hold came for
                  HOLD_SECONDS (a dead man's switch: a button let go, a browser or link gone). 200 {pose, moving,
                  started, vendor}; 409 {reason, state} when refused: estop, fault, not_powered, driver_not_running, restarting, claimed (a Run
                  holds the robot), in_use, maintenance: <why>, vendor_monitor_down; a hold refused while the robot
                  moves stops it. 503 when the vendor's web server failed. The vendor checks the robot's state, its
                  control source and permission itself.

What the robot should be is its desired state (user 11-15: "RLark keeps a state: whether the user wants the robot
powered with its driver running or not, also while the robot is in no Run; when the driver is found down and the user
wants it up, RLark starts it"). It is RLark's, on the robot's Node object:

    label       rlark.io/robot-power   off | powered | ready (powered with the driver running), optionally followed
                                       by "." and when it was set ("off.1794729000": the node page writes it so, and
                                       setting the same state again is a new value). The management plane's label
                                       (set on its node page or through the Gateway; it reaches the Node by RLark's
                                       label sync). Without it a robot is wanted ready: one that just joined is
                                       powered and its driver started with nobody pressing anything.
    label       rlark.io/power-desired the desired state in force: the label's, or ready since a Run used the robot
    label       rlark.io/power-state   what the robot is now: off | powered | ready | estop | fault | unknown
    annotation  rlark.io/power         {"desired", "state", "waiting": why it is not converging now, "recovered": the
                                       last times this service brought the robot back to its desired state,
                                       "errors": the vendor's codes of level 2 (error) the state is fault for}
    label       rlark.io/robot-claim   the Run that holds the robot (the management plane's label, granted, renewed
                                       and ended by the Gateway). Mirrored to the host's /run/rlark/s1u/claim as
                                       {"holder"}, no file while nobody holds the robot, within a moment of its
                                       change: the Node is watched (watch).

This service converges the robot on it every few seconds, for as long as the x86 is up (converge):
- up, always: wanted powered or ready and the motors are off, they are powered; wanted ready and the driver is down
  (it crashed, someone stopped it at the vendor's own page), it is started. Each such recovery is recorded. Not while
  the emergency stop is pressed or the hardware reports an error: the robot stays in that named state and converges
  once it is cleared. An attempt that does not take is repeated after 30 s, then 60 s, up to 10 minutes.
- down, only when a person lowers the desired state: the driver is stopped, the motors powered off, once, as soon as no
  other component marks the robot in use. A robot wanted off or powered that is found higher is otherwise left alone.
- a Run that uses the robot brings it up whatever is desired (the device layer's bring_up on the socket when a
  Station attaches), and from then on the robot is wanted ready, after the Run too (user 11-15), until a person sets
  its desired state at RLark again.
- for a maintenance (the node tuner's: a restart of the robot or of kubelet, the Orin's switch of a payload), the
  robot is lowered and kept from sessions without its desired state being touched. The tuner's lease on the host
  (/run/rlark/s1u/maintenance: kind, power, why, held, until) says so: while it stands every press that would raise
  the robot is refused (409 "maintenance: <why>": a session that attaches is told so), and once it is held the robot
  is converged on the lower of its desired state and the lease's "power", whoever marks it in use (the tuner took
  the lease while nobody did, and nobody was let on since). When the lease ends (the maintenance is over, the host
  restarted, the tuner did not renew it), the robot is brought back to its desired state: one a person powered off
  stays off.
- a restart the Run that holds the robot asks for at RLark (the Node's annotation rlark.io/robot-restart: {id,
  level, holder}, written by the Gateway's POST nodes/{name}/robot-restart; restart_step) runs once per id (the last
  one is kept on the host before it runs, so a restart of this service never runs it again), its desired state
  untouched: the driver stopped and the arms let sink (the node profile's S1U_DRIVER_STOP_SECONDS),
  for level power the motors off for POWER_OFF_SECONDS, then brought up again to ready. Only while the claim names
  that holder and every component's mark under /run/rlark/in-use names its Run ("... Run <holder> ...", the holder
  not followed or preceded by a word character, "." or "-"), and no
  maintenance holds the robot; otherwise it fails with the reason. Nothing else lowers or raises the robot meanwhile
  (the convergence waits, a press is refused "restarting"). Its progress is the annotation rlark.io/power.restart,
  {id, level, holder, phase: lowering | raising | done | failed, error, at}, and "restart" in rlark.io/power.
- not up before the tuner's first word since the host booted (no lease file yet): a robot whose node profile needs
  a restart, a new one's first of all, gets it before its motors are ever on.
Powering a robot off for good is done at RLark: with the desired state still ready, a power-off at the vendor's page
is recovered from. The last desired state seen is kept on the host (S1U_POWER_STATE) for while the API server is away.
On the socket, a press that lowers the robot below its desired state is refused (409 desired).
"""
import json
import os
import re
import signal
import socketserver
import ssl
import threading
import time
from http.client import HTTPException
from http.server import BaseHTTPRequestHandler
from pathlib import Path
from urllib.request import Request, urlopen

SERVICES = {"power_on": ("power_command", True), "power_off": ("power_command", False),
            "driver_on": ("driver_command", True), "driver_off": ("driver_command", False),
            "restart": ("reboot_command", True)}
EMERGENCY_STOP, DRIVER_NORMAL = "12200100", "13000000"
POWER_OFF_SECONDS = 10.0  # the vendor's page: "关节电机下电后需等待10s后方可上电"
CODES_SECONDS = 1.5  # a status code is its component's this long after its last publication (vendor: 0.8 s)
TOPIC = "/astribot_error_code/multi_error_code"
IN_USE = Path("/in-use")  # the host's /run/rlark/in-use
MAINTENANCE = Path("/host/maintenance")  # the node tuner's lease, in the host's /run/rlark/s1u (node_tuner.py)
CLAIM_FILE = Path("/host/claim")  # the Run that holds the robot, in the host's /run/rlark/s1u
RAISES = ("power_on", "driver_on", "bring_up")
BRING_UP_SECONDS = 60.0
SHUT_DOWN_SECONDS = 30.0
# The desired states, and what each button needs the robot to be wanted at most (a press below the desired state is
# RLark's to make, by lowering it).
DESIRED = ("off", "powered", "ready")
LOWERS = {"driver_off": "powered", "power_off": "off", "shut_down": "off", "restart": "off"}
RETRY_SECONDS = (30.0, 600.0)
POSES = {"home": "home_pose"}  # the vendor page's poses by our name
HOLD_SECONDS = 0.5  # a moving pose stops this long after the last hold
VENDOR_WEB_PORT = 5141
ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"
ANNOTATION, STATE, WANTED = "rlark.io/power", "rlark.io/power-state", "rlark.io/robot-power"
IN_FORCE, CLAIM = "rlark.io/power-desired", "rlark.io/robot-claim"
STATION, STATION_MARK = "rlark.io/station", "rlark.io/station-mark-set"
RESTART, RESTARTED = "rlark.io/robot-restart", "rlark.io/power.restart"
RESTARTING = ("lowering", "raising")  # the phases of a restart under way


def robot_state(codes, powered_off_s=None, driver_stop_wait_s=0.0):
    """The robot as the vendor's page reads it from the status ``codes`` (None: its monitor is not publishing).
    ``powered_off_s``: seconds since this service powered the motors off; ``driver_stop_wait_s``: seconds until the
    motors may be powered off after the driver that was running."""
    if codes is None:
        return dict(monitor=False, powered=False, driver=False, emergency_stop=False, ready=False, errors=[],
                    power_level=None, driver_level=None, power_wait_s=0.0, driver_stop_wait_s=0.0)

    def level(module):
        return max((int(code[2]) for code in codes if code.startswith(module)), default=None)

    wait = 0.0 if powered_off_s is None else max(0.0, POWER_OFF_SECONDS - powered_off_s)
    errors = sorted(code for code in codes if code[2] == "2")
    return dict(monitor=True, powered=level("12") is not None, driver=level("13") is not None,
                emergency_stop=EMERGENCY_STOP in codes, ready=DRIVER_NORMAL in codes and not errors, errors=errors,
                power_level=level("12"), driver_level=level("13"), power_wait_s=round(wait, 1),
                driver_stop_wait_s=round(driver_stop_wait_s, 1))


def marks():
    """Each component's mark under /run/rlark/in-use: {name: why}."""
    return {path.name: path.read_text().strip()[:200] for path in sorted(IN_USE.glob("*"))} if IN_USE.is_dir() else {}


def in_use():
    """Why the robot is in use now (each component's mark), or ""."""
    return "; ".join("%s: %s" % mark for mark in marks().items())


def restart_refusal(holder, lease):
    """Why the restart ``holder`` asked for is not run, or "": the claim is another's, a mark is not its Run's, or a
    maintenance holds the robot."""
    held = json.loads(CLAIM_FILE.read_text())["holder"] if CLAIM_FILE.exists() else None
    if held != holder:
        return "the robot is held by %s, not by %s" % (held or "nobody", holder)
    others = {name: why for name, why in marks().items() if not re.search(r"(?<![\w.-])Run %s(?![\w.-])" % re.escape(holder), why)}
    if others:
        return "in use by others: %s" % "; ".join("%s: %s" % mark for mark in others.items())
    return "maintenance: %s" % lease["why"] if lease and lease["kind"] else ""


def maintenance():
    """The node tuner's lease as it stands now: None before the tuner's first word since the host booted, else
    {"kind": "" (no maintenance) | host | kubelet | payload, "power", "why", "held"}. A lease the tuner has not
    renewed until its "until" is over."""
    if not MAINTENANCE.exists():
        return None
    lease = json.loads(MAINTENANCE.read_text())
    return lease if lease["kind"] and time.time() < lease["until"] else dict(kind="")


def refusal(action, state, force=False, busy=""):
    """Why ``action`` is not sent now, or None: the vendor page's conditions, and the robot in use."""
    if not state["monitor"]:
        return "vendor_monitor_down"
    if action == "power_on" and state["power_wait_s"] > 0:
        return "power_off_wait"
    if action in ("power_off", "restart") and busy and not force:
        return "in_use"
    if action == "power_off" and state["driver"] and not force:
        return "driver_running"  # the page asks to stop the driver first, and offers 强制下电
    if action == "power_off" and state["driver_stop_wait_s"] > 0:
        return "driver_stop_wait"  # the arms are still sinking
    if action == "driver_on" and state["power_level"] not in (0, 1):
        return "not_powered"
    return None


class Power:
    """The vendor's status codes and its three services; ``call(service, value)`` answers (success, message)."""

    def __init__(self, call, record=None, driver_stop_seconds=0.0):
        self.call = call
        self.codes, self.codes_at, self.powered_off_at = [], -float("inf"), None
        # When the driver was last seen running, and how long after it the motors keep their power.
        self.driver_at, self.driver_stop_seconds = -float("inf"), driver_stop_seconds
        self.lock = threading.Lock()  # one button at a time
        # The desired state as last read from the Node and the one the robot was last lowered to, on the host (None:
        # in memory only) for while the API server is away and through restarts.
        self.record = record
        self.desired, self.applied = "ready", "ready"
        # The label's value as last read, and the value a Run used the robot under: while the label still reads that,
        # the robot is wanted ready.
        # The label is None until the Node has been read once or a kept state says what it was: a robot whose desired
        # state is not known is left as it is.
        self.label, self.used = None, None
        # The last restart a holder asked for and what became of it (published as RESTARTED), kept with the rest: a
        # request whose id and holder it names is never run again, whatever the Node says. One that was under way when
        # this service stopped did not end. A record from before restarts were kept has none.
        self.restart = None
        if record is not None and record.exists():
            kept = json.loads(record.read_text())
            self.desired, self.applied, self.label, self.used = kept["desired"], kept["applied"], kept["label"], kept["used"]
            self.restart = kept.get("restart")
            if self.restart and self.restart["phase"] in RESTARTING:
                self.restart = dict(self.restart, phase="failed", error="the power service restarted during it",
                                    at=time.time())
        self.saving = threading.Lock()  # the restart's thread writes the record too
        # Why nothing converges now; the recoveries.
        self.waiting, self.recovered = "", []
        # The Node as last watched: {"labels", "annotations"}, empty until it was read (watch).
        self.node = {}
        self.tried, self.pause, self.retry_at, self.converged = None, RETRY_SECONDS[0], 0.0, False

    def keep(self, desired, applied):
        self.desired, self.applied = desired, applied
        self.save()

    def save(self):
        if self.record is None:
            return
        with self.saving:
            self.record.with_suffix(".new").write_text(json.dumps(dict(
                desired=self.desired, applied=self.applied, label=self.label, used=self.used,
                restart=self.restart)) + "\n")
            os.replace(self.record.with_suffix(".new"), self.record)

    def receive(self, text):
        """One publication of the status codes (comma-separated)."""
        self.codes, self.codes_at = [code for code in text.split(",") if code], time.monotonic()
        if any(code.startswith("13") for code in self.codes):
            self.driver_at = self.codes_at

    def state(self):
        fresh = time.monotonic() - self.codes_at < CODES_SECONDS
        off = None if self.powered_off_at is None else time.monotonic() - self.powered_off_at
        return robot_state(self.codes if fresh else None, off,
                           max(0.0, self.driver_stop_seconds - (time.monotonic() - self.driver_at)))

    def send(self, action):
        service, value = SERVICES[action]
        name = "/astribot_system_monitor/" + service
        success, message = self.call(name, value)
        if action == "power_off":
            self.powered_off_at = time.monotonic()
        return dict(sent=name, value=value, success=success, message=message)

    def act(self, action, force=False, reason="", sleep=time.sleep):
        """(HTTP status, body) of one button on the socket. A press that lowers the robot below its desired state is
        refused: the desired state is lowered at RLark. A bring_up above it is a session's."""
        if force and not reason.strip():
            return 400, dict(detail="force needs a reason")
        if action in LOWERS and DESIRED.index(self.desired) > DESIRED.index(LOWERS[action]):
            return 409, dict(reason="desired", desired=self.desired, state=self.state())
        with self.lock:
            state, lease = self.state(), maintenance()
            if self.restarting():
                return 409, dict(reason="restarting", state=state)
            if action in RAISES and lease and lease["kind"]:
                return 409, dict(reason="maintenance: %s" % lease["why"], state=state)
            if action in ("bring_up", "shut_down"):
                try:
                    answer = self.bring_up(sleep) if action == "bring_up" else self.shut_down(sleep, force)
                except Exception as error:
                    return 503, dict(detail="vendor service failed: %s" % error, state=self.state())
                if action == "bring_up" and answer[0] == 200:
                    # A Run uses the robot: wanted ready from now on, until its desired state is set again at RLark.
                    self.used = self.label
                    self.keep("ready", "ready")
                return answer
            refused = refusal(action, state, force, in_use())
            if refused:
                return 409, dict(reason=refused, state=state, in_use=in_use())
            try:
                return 200, dict(self.send(action), state=self.state())
            except Exception as error:  # the service is gone or failed: the operator reads why
                return 503, dict(detail="vendor service failed: %s" % error, state=state)

    def wait(self, check, seconds, sleep):
        """Whether ``check(state)`` holds within ``seconds``."""
        until = time.monotonic() + seconds
        while time.monotonic() < until:
            if check(self.state()):
                return True
            sleep(0.5)
        return check(self.state())

    def shut_down(self, sleep, force=False):
        """The driver stopped and the motors off: the driver first, the motors once the arms have had their time to
        sink, each step waited for up to SHUT_DOWN_SECONDS. Not while the robot is in use, unless forced."""
        state, busy = self.state(), in_use()
        if not state["monitor"]:
            return 409, dict(reason="vendor_monitor_down", state=state)
        if busy and not force:
            return 409, dict(reason="in_use", state=state, in_use=busy)
        if state["driver"]:
            self.send("driver_off")
            if not self.wait(lambda s: not s["driver"], SHUT_DOWN_SECONDS, sleep):
                return 504, dict(detail="driver still running after %d s" % SHUT_DOWN_SECONDS, state=self.state())
        if self.state()["powered"]:
            if not self.wait(lambda s: s["driver_stop_wait_s"] == 0 or not s["monitor"], SHUT_DOWN_SECONDS, sleep):
                return 504, dict(detail="the driver's stop has not settled", state=self.state())
            self.send("power_off")
            if not self.wait(lambda s: not s["powered"], SHUT_DOWN_SECONDS, sleep):
                return 504, dict(detail="still powered after %d s" % SHUT_DOWN_SECONDS, state=self.state())
        return 200, dict(sent="shut_down", value=False, success=True, message="off", state=self.state())

    def bring_up(self, sleep):
        """Powered and the driver ready: power on unless powered, the driver once the motors are on (by itself
        within 5 s where the desired state is ready, else asked for), each step waited for up to BRING_UP_SECONDS."""
        def wait(check, seconds):
            return self.wait(check, seconds, sleep)

        state = self.state()
        refused = None if state["powered"] else refusal("power_on", state)
        if refused:
            return 409, dict(reason=refused, state=state)
        if not state["powered"]:
            self.send("power_on")
            if not wait(lambda s: s["powered"], BRING_UP_SECONDS):
                return 504, dict(detail="not powered within %d s" % BRING_UP_SECONDS, state=self.state())
        if not wait(lambda s: s["driver"], 5.0):
            self.send("driver_on")
        if not wait(lambda s: s["ready"] or s["errors"], BRING_UP_SECONDS):
            return 504, dict(detail="not ready within %d s" % BRING_UP_SECONDS, state=self.state())
        state = self.state()
        if state["errors"]:
            return 504, dict(detail="errors: %s" % ",".join(state["errors"]), state=state)
        return 200, dict(sent="bring_up", value=True, success=True, message="ready", state=state)

    def restarting(self):
        return self.restart is not None and self.restart["phase"] in RESTARTING

    def restart_step(self, request, sleep=time.sleep):
        """A restart the holder asked for (``request``: the Node's RESTART) started on a thread of its own, unless it
        is the last one (kept on the host before it is run) or one runs."""
        if request is None or self.restarting() or self.restart and (self.restart["id"], self.restart["holder"]) == (
                request["id"], request["holder"]):
            return
        with self.lock:
            self.restart = dict(id=request["id"], level=request["level"], holder=request["holder"], phase="lowering",
                                error="", at=time.time())
            self.save()
        print("%s restart %s (%s) for %s" % (time.strftime("%F %T"), request["id"], request["level"], request["holder"]),
              flush=True)
        threading.Thread(target=self.restart_run, args=(maintenance(), sleep), daemon=True).start()

    def restart_run(self, lease, sleep):
        """The restart in self.restart: the driver stopped and the arms let sink, for level power the motors off for
        POWER_OFF_SECONDS, then up to ready. Each step waited for up to SHUT_DOWN_SECONDS (up: bring_up's)."""
        restart = self.restart

        def phase(name, error=""):
            self.restart = dict(restart, phase=name, error=error, at=time.time())
            self.save()
            if error:
                print("%s restart %s failed: %s" % (time.strftime("%F %T"), restart["id"], error), flush=True)

        state = self.state()
        refused = restart_refusal(restart["holder"], lease) or (
            "the vendor's system monitor does not answer" if not state["monitor"] else
            "emergency stop" if state["emergency_stop"] else "")
        if refused:
            return phase("failed", refused)
        try:
            if state["driver"]:
                self.send("driver_off")
                if not self.wait(lambda s: not s["driver"], SHUT_DOWN_SECONDS, sleep):
                    return phase("failed", "the driver still runs %d s after driver_off" % SHUT_DOWN_SECONDS)
            if not self.wait(lambda s: s["driver_stop_wait_s"] == 0, SHUT_DOWN_SECONDS, sleep):
                return phase("failed", "the driver's stop has not settled")
            if restart["level"] == "power" and self.state()["powered"]:
                self.send("power_off")
                if not self.wait(lambda s: not s["powered"], SHUT_DOWN_SECONDS, sleep):
                    return phase("failed", "still powered %d s after power_off" % SHUT_DOWN_SECONDS)
                self.wait(lambda s: s["power_wait_s"] == 0, POWER_OFF_SECONDS + 1.0, sleep)
            phase("raising")
            status, answer = self.bring_up(sleep)
        except Exception as error:  # the vendor's service is gone or failed
            return phase("failed", "vendor service failed: %s" % error)
        if status != 200:
            return phase("failed", answer.get("reason") or answer["detail"])
        phase("done")

    def summary(self):
        """What the robot is now, in a word (the Node's label STATE)."""
        state = self.state()
        if not state["monitor"]:
            return "unknown"
        if state["emergency_stop"]:
            return "estop"
        if state["errors"]:
            return "fault"
        return "ready" if state["driver"] else "powered" if state["powered"] else "off"

    def converge(self, label, busy, now, lease):
        """One step towards the desired state: that of ``label`` (the Node's label WANTED, "<state>" or
        "<state>.<when>"; anything else reads ready), or ready while the label is still the one a Run used the robot
        under; under a held ``lease`` (maintenance()) the lower of it and the lease's. ``busy`` is why the robot is in
        use ("" when it is not), ``now`` a monotonic time. At most one of the vendor's services is called; none while
        ``label`` is None (the Node was never read and nothing was kept)."""
        if label is None:
            self.waiting = "the Node has not been read: the desired state is not known"
            return
        if self.restarting():
            self.waiting = "restart %s: %s" % (self.restart["id"], self.restart["phase"])
            return
        if label != self.used:
            self.used = None  # set again at RLark since the Run: the label's state is in force
        desired = label.split(".")[0] if self.used is None else "ready"
        desired = desired if desired in DESIRED else "ready"
        if desired != self.desired or label != self.label:
            self.label = label
            self.keep(desired, self.applied)
            self.tried = None
        state, self.waiting = self.state(), ""
        if not state["monitor"]:
            self.waiting = "the vendor's system monitor does not answer"
            return
        # What the robot is converged on now: its desired state, or less while a maintenance holds it.
        lowered = bool(lease) and lease.get("held") and DESIRED.index(lease["power"]) < DESIRED.index(desired)
        wanted, desired = desired, lease["power"] if lowered else desired
        with self.lock:
            if desired != self.applied:
                # Lowered by a person, once nothing marks the robot in use, or for a maintenance: the driver, then
                # the motors.
                down = ("driver_off" if state["driver"] and desired != "ready" else
                        "power_off" if state["powered"] and desired == "off" else None)
                if down and busy and not lowered:
                    self.waiting = "in use (%s): %s waits" % (busy, down)
                    return
                if down == "power_off" and state["driver_stop_wait_s"] > 0:
                    self.waiting = "power_off %.0f s from now: the arms sink after the driver's stop" % state["driver_stop_wait_s"]
                    return
                if down:
                    print("%s %s: %s" % (time.strftime("%F %T"), down, "maintenance: %s" % lease["why"] if lowered else
                                         "the desired state is %s" % desired), flush=True)
                    self.send(down)
                    return
                # What it comes back up from was asked for: bringing it up again is no recovery.
                self.keep(wanted, desired)
                self.converged = False
            up = ("power_on" if desired != "off" and not state["powered"] else
                  "driver_on" if desired == "ready" and not state["driver"] else None)
            if lease and lease["kind"]:
                self.waiting = "maintenance: %s" % lease["why"]
                return
            if up is None:
                self.tried, self.converged = None, True
                return
            if lease is None:
                self.waiting = "the node tuner has not said yet whether this boot needs a maintenance"
                return
            if state["emergency_stop"] or state["errors"]:
                self.waiting = "emergency stop" if state["emergency_stop"] else "errors %s" % ",".join(state["errors"])
                return
            if up == "driver_on" and state["power_level"] not in (0, 1) or up == "power_on" and state["power_wait_s"] > 0:
                self.waiting = "the motors are not ready for %s yet" % up
                return
            if up != self.tried:  # the step before took: this one is a first attempt
                self.tried, self.pause, self.retry_at = up, RETRY_SECONDS[0], 0.0
            if now < self.retry_at:
                self.waiting = "%s did not take, again in %d s" % (up, self.retry_at - now)
                return
            print("%s %s: the desired state is %s and the robot is %s" % (
                time.strftime("%F %T"), up, desired, self.summary()), flush=True)
            if self.converged:  # it was as desired and fell from it: a recovery, for people to read
                self.recovered = (self.recovered + [dict(at=time.time(), action=up, was=self.summary())])[-5:]
            self.send(up)
            self.retry_at, self.pause = now + self.pause, min(self.pause * 2, RETRY_SECONDS[1])


class Pose:
    """The vendor page's hold-to-move poses: ``post(message)`` sends one message to the vendor's web server and
    answers its reply. One pose moves at a time; a watch thread stops it once its holds stop coming."""

    def __init__(self, power, post):
        self.power, self.post = power, post
        self.lock = threading.Lock()
        self.moving, self.until = None, 0.0  # the pose moving now, and when it stops without another hold

    def refusal(self):
        state, lease = self.power.state(), maintenance()
        if not state["monitor"]:
            return "vendor_monitor_down", state
        if state["emergency_stop"]:
            return "estop", state
        if state["errors"]:
            return "fault", state
        if not state["powered"]:
            return "not_powered", state
        if not state["driver"]:
            return "driver_not_running", state
        if self.power.restarting():
            return "restarting", state
        if CLAIM_FILE.exists():
            return "claimed", state
        if in_use():
            return "in_use", state
        if lease and lease["kind"]:
            return "maintenance: %s" % lease["why"], state
        return None, state

    def act(self, pose, action):
        """(HTTP status, body) of one hold or stop."""
        with self.lock:
            if action == "stop":
                return 200, dict(pose=pose, moving=False, started=False, vendor=self.stop())
            refused, state = self.refusal()
            if refused:
                self.stop()
                return 409, dict(reason=refused, state=state)
            started = self.moving is None
            self.moving = pose
            if not started:
                self.until = time.monotonic() + HOLD_SECONDS
                return 200, dict(pose=pose, moving=True, started=False, vendor=None)
            threading.Thread(target=self.watch, daemon=True).start()
            try:
                vendor = self.post(dict(msg_id="pose_running", data=dict(pose=POSES[pose])))
            except Exception as error:  # it may have reached the robot: the watch sends pose_stop at once
                self.until = 0.0
                return 503, dict(detail="vendor web server failed: %s" % error)
            # Counted from the vendor's reply: holds queued behind a slow start are not late.
            self.until = time.monotonic() + HOLD_SECONDS
            return 200, dict(pose=pose, moving=True, started=True, vendor=vendor)

    def stop(self):
        """pose_stop when a pose moves (the lock held); the vendor's reply, or None. A pose_stop that failed leaves the
        pose moving: the watch sends it again."""
        if self.moving is None:
            return None
        reply = self.post(dict(msg_id="pose_stop"))
        self.moving = None
        return reply

    def watch(self):
        while True:
            time.sleep(0.05)
            with self.lock:
                if self.moving is None:
                    return
                if time.monotonic() < self.until:
                    continue
                print("%s pose_stop: no hold for %.1f s" % (time.strftime("%F %T"), HOLD_SECONDS), flush=True)
                try:
                    self.stop()
                except Exception as error:  # sent again on the next look
                    print("%s pose_stop failed: %s" % (time.strftime("%F %T"), error), flush=True)


class Server(socketserver.ThreadingMixIn, socketserver.UnixStreamServer):
    daemon_threads = True


def handler(power, pose):
    class Handler(BaseHTTPRequestHandler):
        def reply(self, status, body):
            data = json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if self.path != "/power":
                return self.reply(404, dict(detail="no such path"))
            labels, annotations = power.node.get("labels", {}), power.node.get("annotations", {})
            mark = annotations.get(STATION_MARK)
            self.reply(200, dict(power.state(), restart=power.restart, desired=power.desired, waiting=power.waiting,
                                 station=labels.get(STATION), station_mark=json.loads(mark) if mark else None))

        def do_POST(self):
            if self.path not in ("/power", "/pose"):
                return self.reply(404, dict(detail="no such path"))
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
            if self.path == "/pose":
                return self.do_pose(body)
            if body.get("action") not in (*SERVICES, "bring_up", "shut_down"):
                return self.reply(400, dict(detail="unsupported action"))
            status, answer = power.act(body["action"], bool(body.get("force")), str(body.get("reason") or ""))
            # Every press and what became of it; who pressed is RLark's record (the header it sets).
            print("%s %s force=%s reason=%r by %s: %s %s" % (
                time.strftime("%F %T"), body["action"], bool(body.get("force")), body.get("reason") or "",
                self.headers.get("X-Remote-User", "?"), status, answer.get("reason") or answer.get("detail") or "sent"),
                flush=True)
            self.reply(status, answer)

        def do_pose(self, body):
            if body.get("pose") not in POSES or body.get("action") not in ("hold", "stop"):
                return self.reply(400, dict(detail="pose must be home and action hold or stop"))
            try:
                status, answer = pose.act(body["pose"], body["action"])
            except Exception as error:  # pose_stop failed: the operator reads why
                status, answer = 503, dict(detail="vendor web server failed: %s" % error)
            # A movement's start and end, and every refusal; the holds between are not.
            if body["action"] == "stop" or status != 200 or answer["started"]:
                print("%s pose %s %s by %s: %s %s" % (
                    time.strftime("%F %T"), body["pose"], body["action"], self.headers.get("X-Remote-User", "?"), status,
                    answer.get("reason") or answer.get("detail") or "sent"), flush=True)
            self.reply(status, answer)

        def address_string(self):
            return "unix"

        def log_message(self, *arguments):
            pass

    return Handler


def node(query, patch=None, timeout=10):
    """The API server's reply on nodes (``query`` follows /api/v1/nodes) with the Pod's service account; ``patch``:
    labels and annotations to set on the Node named."""
    request = Request(
        "https://%s:%s/api/v1/nodes%s" % (os.environ["KUBERNETES_SERVICE_HOST"], os.environ["KUBERNETES_SERVICE_PORT"], query),
        data=None if patch is None else json.dumps(dict(metadata=patch)).encode(), method="PATCH" if patch else "GET",
        headers={"Authorization": "Bearer " + open(ACCOUNT + "/token").read(), "Content-Type": "application/merge-patch+json"})
    return urlopen(request, context=ssl.create_default_context(cafile=ACCOUNT + "/ca.crt"), timeout=timeout)


def vital(target, *args):
    """``target`` on a thread of its own whose end, for whatever reason, ends the process: kubelet then starts the
    container again, instead of the claim file or the desired state staying as they were with nothing following them."""
    def run():
        try:
            target(*args)
        finally:
            os._exit(1)
    threading.Thread(target=run, daemon=True).start()


def watch(seen):
    """This robot's Node as the API server tells each change of it (a watch of this one Node, as kubelet's own): its
    labels into ``seen``, and the Run that holds the robot (the label CLAIM) into CLAIM_FILE for whatever drives the
    robot, within a moment of the change. Each watch starts with the Node as it is and ends after 30 s (a connection
    cut without a word is found within 35 s); one that fails is tried again 5 s later. While the API server is away
    both stay as last read: a claim ends at RLark, not here."""
    while True:
        try:
            with node("?watch=true&fieldSelector=metadata.name%%3D%s&timeoutSeconds=30" % os.environ["NODE_NAME"],
                      timeout=35) as events:
                for line in events:
                    event = json.loads(line)
                    if event["type"] not in ("ADDED", "MODIFIED"):
                        continue
                    seen["labels"] = labels = event["object"]["metadata"].get("labels", {})
                    seen["annotations"] = event["object"]["metadata"].get("annotations", {})
                    if CLAIM in labels:
                        CLAIM_FILE.with_suffix(".new").write_text(json.dumps(dict(holder=labels[CLAIM])))
                        CLAIM_FILE.with_suffix(".new").replace(CLAIM_FILE)
                    else:
                        CLAIM_FILE.unlink(missing_ok=True)
        except (OSError, ValueError, HTTPException) as error:  # IncompleteRead: the stream cut in the middle
            print("the Node is not watched, its claim and desired state stay as last read: %s" % error, flush=True)
            time.sleep(5.0)


def follow(power):
    """Every 5 s: a restart asked for at RLark started, the desired state from the Node as last watched (the last one
    seen while the API server is away), one step towards it, and what the robot is, why it is not converging and the
    restart's progress published when they change."""
    published, seen = None, power.node
    vital(watch, seen)
    while True:
        label = seen["labels"].get(WANTED, "") if "labels" in seen else power.label
        if "annotations" in seen and RESTART in seen["annotations"]:
            power.restart_step(json.loads(seen["annotations"][RESTART]))
        try:
            power.converge(label, in_use(), time.monotonic(), maintenance())
        except Exception as error:  # the vendor's service is gone or failed: said, and tried again
            power.waiting = "vendor service failed: %s" % error
        state = power.state()
        now = (power.summary(), power.desired, json.dumps(dict(
            desired=power.desired, used_by_a_run=power.used is not None, state=power.summary(), waiting=power.waiting,
            recovered=power.recovered, errors=state["errors"], restart=power.restart)))
        if now != published:
            if published is None or now[0] != published[0]:
                # Which of the vendor's codes made the state what it is: a fault or an emergency stop names them.
                print("%s state %s%s" % (time.strftime("%F %T"), now[0], " (codes %s)" % ",".join(state["errors"])
                                         if state["errors"] else ""), flush=True)
            annotations = {ANNOTATION: now[2]}
            if power.restart:  # never written empty: a restart is not forgotten at the Node either
                annotations[RESTARTED] = json.dumps(power.restart)
            try:
                node("/" + os.environ["NODE_NAME"],
                     dict(labels={STATE: now[0], IN_FORCE: now[1]}, annotations=annotations)).close()
                published = now
            except OSError as error:
                print("the Node is not written: %s" % error, flush=True)
        time.sleep(5.0)


def main():
    lines = (line.split("=", 1) for line in open(os.environ["S1U_NODE_PROFILE"]).read().splitlines()
             if "=" in line and line[0] != "#")
    profile = {key: value.strip('"') for key, value in lines}
    os.environ.update(ROS_MASTER_URI="http://%s:11311" % profile["S1U_ROS_IP_ADDRESS"], ROS_IP=profile["S1U_ROS_IP_ADDRESS"])
    import rospy
    from std_msgs.msg import String
    from std_srvs.srv import SetBool

    def call(name, value):
        rospy.wait_for_service(name, timeout=2.0)
        reply = rospy.ServiceProxy(name, SetBool)(value)
        return reply.success, reply.message

    record = Path(os.environ["S1U_POWER_STATE"])
    record.parent.mkdir(parents=True, exist_ok=True)
    power = Power(call, record, float(profile["S1U_DRIVER_STOP_SECONDS"]))
    # The socket first: with the robot's master down the state says so (monitor false), it is not missing.
    path = Path(os.environ["S1U_SERVICE_SOCKET"])
    path.parent.mkdir(parents=True, exist_ok=True)
    path.unlink(missing_ok=True)
    vendor = "http://%s:%d/msg" % (profile["S1U_ROS_IP_ADDRESS"], VENDOR_WEB_PORT)

    def post(message):
        request = Request(vendor, data=json.dumps(message).encode(), headers={"Content-Type": "application/json"})
        with urlopen(request, timeout=2.0) as response:
            text = response.read().decode()
        return json.loads(text) if text.strip().startswith(("{", "[")) else text

    def halt(why):
        """pose_stop whatever this process knows: a pose started by a process before it is stopped too."""
        try:
            post(dict(msg_id="pose_stop"))
        except Exception as error:
            print("%s pose_stop at %s failed: %s" % (time.strftime("%F %T"), why, error), flush=True)

    # A pose held under a process that died has no dead man's switch left: it stops now, and with this process.
    halt("start")
    signal.signal(signal.SIGTERM, lambda *_: (halt("SIGTERM"), os._exit(0)))
    server = Server(str(path), handler(power, Pose(power, post)))
    os.chmod(path, 0o600)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    rospy.init_node("s1u_power", disable_signals=True)  # waits for the master
    rospy.Subscriber(TOPIC, String, lambda message: power.receive(message.data), queue_size=1)
    vital(follow, power)
    print("power of this robot on %s" % path, flush=True)
    threading.Event().wait()


if __name__ == "__main__":
    main()
