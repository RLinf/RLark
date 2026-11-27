"""The robot's cameras, a container of the Addon s1u-robot on every S1U robot x86 (video): the one reader of the
cameras for whoever looks at the robot, and their keeper. Python 3.8.

It subscribes each camera of the robot's body (its Node's label rlark.io/robot-body, bodies.json) from start to stop,
whether anyone watches or a Run uses the robot: the vendor's compressed topic
/astribot_camera/<camera>/color_compress/compressed, received raw (rospy.AnyMsg, no decode), of which it keeps the
newest JPEG, when it arrived and the rate.

Read-only HTTP on the node's address (S1U_NODE_ADDRESS, the site tunnel's), port 18891; RLark's Gateway reaches it
through the cluster's API server (the Pod's proxy) and decides who may look:
    GET /cameras                        {state, cameras: {view: {state, fps, age_seconds, step, attempts,
                                        next_attempt_at, last_action, last_result, diagnosis, usb, usb_advice}},
                                        updated_at}
    GET /cameras/<view>.jpg             the newest JPEG (X-Age-Seconds); 404 a view the body lacks, 503 no frame yet
    GET /cameras/<view>/stream?fps=N    multipart/x-mixed-replace of the camera's own JPEGs, N 1..30 (10); 404, 503 alike
    GET /cameras/wall.jpg               the four views as one 256 x 144 image: head, torso / wrist_left, wrist_right,
    GET /cameras/wall/stream?fps=N      each 128 x 72, a view that is missing or stale black; N 1..10 (5)
Only the wall decodes (OpenCV, at a quarter of the size in libjpeg itself), and only while someone asks for it.

A camera is ok | slow (under 10 frames a second for 30 s) | stale (no frame for 5 s) | absent (none since this
started). Health is the topic alone: the vendor's /astribot_error_code/multi_error_code keeps only the last of the
camera driver's codes and is not read. A camera that is stale or absent for 30 s (longer than the blank of a driver
restart, anyone's) is brought back by a ladder (Recovery), each camera on its own place of it, each step an Event on
the Node:
    1. the vendor's camera driver restarted (/astribot_system_monitor/camera_control_command False, 2 s, True: all
       cameras blank for about 25 s), up to 3 times, 60 s apart doubling to 900 s; one restart counts for every camera
       down that has restarts to go. It is also how the driver starts after a boot: the vendor's monitor starts it on
       True. A slow camera gets this step only, once an hour.
    2. its USB device reset on the Orin by the orin container, then the driver restarted once more, which counts for
       every other camera down that has restarts to go (s1u_01 11-13 22:40: the head camera stalled inside the
       Orbbec SDK and no driver restart brought it back). A request the frames came back before is withdrawn. A
       reset that did not run (ssh, the Orin or the orin container away) is asked again after the camera's pause,
       nothing restarted; a camera the Orin's USB lacks is stuck at once (NO_DEVICE).
    3. stuck: 相机需要断电或 USB 复位. A camera that stalls again within 5 minutes of its USB reset is stuck at once:
       疑似硬件 (s1u-03 11-21: the head camera's colour stream stopped 25 s to 3 min after each reset).
A camera's ladder starts over once it has sent for 5 minutes: one that comes back for a moment and stalls again goes
on up it. A stuck camera leaves the ladder until it has sent for 5 minutes again or moved to another USB port
(rewired; a reset keeps the port), when its ladder starts over; the others keep theirs. Its driver is restarted once
an hour, and only while nothing marks the robot in use (/run/rlark/in-use): each restart blanks the cameras that work
(s1u-03 11-21: hourly under any Run, 89 restarts of every camera in 26 h), and it brought the head back for minutes
to half an hour at a time. Its Event CameraStuck carries what
the orin container found on the Orin (diagnosis: on the USB or not and where, the vendor's camera process, an Orbbec
camera's colour and depth streams in the SDK's log). Nothing is done in the first 90 s.

The orin container is asked through the Pod's shared directory (S1U_CAMERA_DIRECTORY): <action>-request {id,
cameras: the vendor's names}, taken back once answered or no longer waited for, and its <action>-result {id, ...,
finished_at}, for reset {ok, detail} and diagnose {cameras: {name: diagnosis}}; it writes each camera's USB position
to usb ({name: {path, device, speed, hubs}}), which the report carries, with advice for a camera on a hub with fewer
cameras than another hub on its root port (usb_advice).

The state is on the Node, written when it changes: label rlark.io/camera-state ok | recovering | degraded (slow
cameras only) | stuck, annotation rlark.io/cameras the JSON of GET /cameras. A restart of the robot's ROS master ends
this process (rospy does not subscribe again at a new master): kubelet starts it afresh, stuck cameras with it.
"""

import json
import os
import re
import ssl
import struct
import threading
import time
import uuid
from collections import defaultdict, deque
from concurrent.futures import ThreadPoolExecutor
from concurrent.futures import TimeoutError as Unanswered
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse
from urllib.request import Request, urlopen
from xmlrpc.client import ServerProxy

import cv2
import numpy

import body_observer
from power import in_use, vital

# Each view by the vendor driver's name of its camera, in the wall's order.
VENDOR = {"head": "head_rgbd", "torso": "torso_rgbd", "wrist_left": "left_wrist_rgbd", "wrist_right": "right_wrist_rgbd"}
VIEWS = tuple(VENDOR)
TOPIC = "/astribot_camera/%s/color_compress/compressed"
SERVICE = "/astribot_system_monitor/camera_control_command"
PORT = 18891
BOUNDARY = "s1ucamera"
CELL = (128, 72)
STREAMS = 16  # open streams at most

STALE_SECONDS = 5.0
SLOW_FPS, SLOW_SECONDS, SLOW_RESTART_SECONDS = 10.0, 30.0, 3600.0
GRACE_SECONDS = 90.0
SETTLE_SECONDS = 30.0  # cameras down for less are left alone: a driver restart blanks them for about 25 s
HEALTHY_SECONDS = 300.0  # a camera's ladder starts over once it has sent this long, and a stuck one leaves stuck
STUCK_RETRY_SECONDS = 3600.0  # stuck cameras' driver restarted this often while nothing marks the robot in use
RESTART_SECONDS, RESTARTS = (60.0, 900.0), 3
CALL_SECONDS = 30.0  # the vendor's monitor answers a call within this, or the call failed
ANSWER_SECONDS = 300.0  # the orin container's answer is waited for this long
STUCK = "相机需要断电或 USB 复位"
SUSPECT = "疑似硬件：USB 复位后 %d 秒又停，需要断电或现场检查接线"
NO_DEVICE = "Orin 的 USB 上没有这路相机（camera_info.json 的序列号），USB 复位没做"

ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"
STATE, ANNOTATION = "rlark.io/camera-state", "rlark.io/cameras"


def jpeg_of(raw):
    """The JPEG in a serialized sensor_msgs/CompressedImage: after the header (seq, stamp, frame_id) and the format."""
    (length,) = struct.unpack_from("<I", raw, 12)
    offset = 16 + length
    (length,) = struct.unpack_from("<I", raw, offset)
    offset += 4 + length
    (length,) = struct.unpack_from("<I", raw, offset)
    return raw[offset + 4:offset + 4 + length]


def capture_time_ms(raw, received_ms):
    """ROS header.stamp in Unix milliseconds, only when plausible on the robot clock."""
    if len(raw) < 12:
        return None
    seconds, nanos = struct.unpack_from("<2I", raw, 4)
    if not seconds or nanos >= 1_000_000_000:
        return None
    stamp = seconds * 1000 + nanos / 1_000_000
    return stamp if -50 <= received_ms - stamp <= 5000 else None


class Cameras:
    """The body's cameras' newest frames, and what each camera is."""

    def __init__(self, views, clock=time.monotonic, wall=time.time):
        self.views = tuple(view for view in VIEWS if view in views)
        self.clock = clock
        self.wall_clock = wall
        self.lock = threading.Lock()
        self.framed = threading.Condition(self.lock)
        # samples: (time, frames so far) of the last five seconds, one a second; slow_since: since when under SLOW_FPS.
        self.frames = {view: dict(raw=None, received=None, sequence=0, samples=deque(maxlen=6), slow_since=None)
                       for view in self.views}
        self.composed = (None, None)  # the wall as last made: (when, JPEG)

    def receive(self, view, raw):
        with self.lock:
            frame = self.frames[view]
            frame.update(raw=raw, received=self.clock(), sequence=frame["sequence"] + 1,
                         capture_ms=capture_time_ms(raw, self.wall_clock() * 1000))
            self.framed.notify_all()

    def fps(self, view):
        samples = self.frames[view]["samples"]
        if len(samples) < 2:
            return 0.0
        return round((samples[-1][1] - samples[0][1]) / (samples[-1][0] - samples[0][0]), 1)

    def sample(self, now):
        """Once a second: each camera's rate over the last five, and since when a camera that sends is under SLOW_FPS
        (one that sends nothing is stale, not slow: after it is back it has SLOW_SECONDS to reach its rate)."""
        with self.lock:
            for view, frame in self.frames.items():
                frame["samples"].append((now, frame["sequence"]))
                sending = frame["received"] is not None and now - frame["received"] <= STALE_SECONDS
                frame["slow_since"] = (frame["slow_since"] or now) if sending and self.fps(view) < SLOW_FPS else None

    def states(self, now):
        with self.lock:
            return {view: "absent" if frame["received"] is None else
                    "stale" if now - frame["received"] > STALE_SECONDS else
                    "slow" if frame["slow_since"] is not None and now - frame["slow_since"] >= SLOW_SECONDS else "ok"
                    for view, frame in self.frames.items()}

    def age(self, view, now):
        received = self.frames[view]["received"]
        return None if received is None else round(max(0.0, now - received), 2)

    def jpeg(self, view):
        """(the newest JPEG, its age), (None, None) before the camera's first frame."""
        with self.lock:
            raw = self.frames[view]["raw"]
            return (None, None) if raw is None else (jpeg_of(raw), self.age(view, self.clock()))

    def next(self, view, sequence):
        """(JPEG, age, sequence, capture timestamp) of the newest frame after ``sequence``; a camera that
        stopped gives its last frame again each second, so a viewer sees its age grow."""
        with self.lock:
            frame = self.frames[view]
            self.framed.wait_for(lambda: frame["sequence"] != sequence, timeout=1)
            return jpeg_of(frame["raw"]), self.age(view, self.clock()), frame["sequence"], frame["capture_ms"]

    def wall(self):
        """The four views as one JPEG, made at most ten times a second however many ask; a view without a fresh
        frame (or whose frame does not decode) black."""
        now = self.clock()
        with self.lock:
            if self.composed[0] is not None and now - self.composed[0] < 0.1:
                return self.composed[1]
            fresh = {view: frame["raw"] for view, frame in self.frames.items()
                     if frame["received"] is not None and now - frame["received"] <= STALE_SECONDS}
        canvas = numpy.zeros((2 * CELL[1], 2 * CELL[0], 3), numpy.uint8)
        for index, view in enumerate(VIEWS):
            image = cv2.imdecode(numpy.frombuffer(jpeg_of(fresh[view]), numpy.uint8), cv2.IMREAD_REDUCED_COLOR_4) if view in fresh else None
            if image is not None:
                x, y = index % 2 * CELL[0], index // 2 * CELL[1]
                canvas[y:y + CELL[1], x:x + CELL[0]] = cv2.resize(image, CELL, interpolation=cv2.INTER_AREA)
        jpeg = cv2.imencode(".jpg", canvas, [cv2.IMWRITE_JPEG_QUALITY, 70])[1].tobytes()
        with self.lock:
            self.composed = (now, jpeg)
        return jpeg


class Recovery:
    """The ladder that brings stale and absent cameras back, each camera on its own place of it. ``restart`` restarts
    the vendor's camera driver (it raises when the vendor's service is away); ``event(reason, message, kind)`` tells
    the Node; ``directory`` is shared with the orin container."""

    def __init__(self, cameras, restart, directory, event, wall=time.time, busy=in_use):
        self.cameras, self.restart, self.event, self.wall, self.busy = cameras, restart, event, wall, busy
        self.directory = Path(directory)
        self.started = cameras.clock()
        self.records = {view: dict(step=0, attempts=0, next_attempt_at=None, last_action=None, last_result=None,
                                   diagnosis=None, usb=None, usb_advice=None) for view in cameras.views}
        self.slow_at = None  # when the driver was last restarted for a slow camera
        self.since = {view: (False, self.started) for view in cameras.views}  # (down, since when) of each camera
        # Each camera's place on the ladder, from its first step until it has sent for HEALTHY_SECONDS.
        self.ladder = defaultdict(lambda: dict(step=0, attempts=0, pause=RESTART_SECONDS[0], next_at=0.0, reset=False))
        self.stuck = set()  # the cameras the ladder gave up on
        self.retried_at = None  # when the stuck cameras' driver was last restarted, or a camera given up
        self.reset_at = {}  # when each camera's USB device was last reset
        self.request = None  # the USB reset asked for and not answered yet: (id, when, views)
        self.diagnosing = None  # the stuck cameras' facts asked for and not answered yet: (id, when, views)
        self.usb = {}  # each camera's USB device as the orin container last found it, by the vendor's name
        self.paths = {}  # each camera's USB path as last found on the bus
        for action in ("reset", "diagnose"):  # an earlier run's request, which nobody waits for
            (self.directory / (action + "-request")).unlink(missing_ok=True)

    def mark(self, views, now, timed=True, **fields):
        """What the ladder did for ``views``, as GET /cameras says it; ``timed``: it acts again at their next_at."""
        for view in views:
            place = self.ladder[view]
            self.records[view].update(fields, step=place["step"], attempts=place["attempts"], next_attempt_at=(
                round(self.wall() + place["next_at"] - now) if timed else None))

    def ask(self, action, views, now):
        """The orin container asked to ``action`` (reset, diagnose) the cameras ``views``: (id, when, views)."""
        asked = (uuid.uuid4().hex, now, views)
        new = self.directory / (action + "-request.new")
        new.write_text(json.dumps(dict(id=asked[0], cameras=[VENDOR[view] for view in views])) + "\n")
        new.replace(self.directory / (action + "-request"))
        return asked

    def answer(self, action, asked, now):
        """The orin container's result for ``asked``: None while it is awaited, {} when it did not come in time. The
        request is taken back once answered or no longer waited for: each is done once."""
        try:
            result = json.loads((self.directory / (action + "-result")).read_text())
        except (OSError, ValueError):  # none yet, or being written
            result = {}
        if result.get("id") != asked[0]:
            if now - asked[1] < ANSWER_SECONDS:
                return None
            result = {}
        (self.directory / (action + "-request")).unlink(missing_ok=True)
        return result

    def locate(self):
        """Each camera's USB device as the orin container wrote it, and advice for one that hangs on a hub with fewer
        cameras than another hub on its root port (s1u-03 11-21: the head's colour stream stalled behind two hubs on
        the root port of the torso and the wrists, which share a hub; on s1u-01 and s1u-02 the head has a root port of
        its own)."""
        try:
            usb = json.loads((self.directory / "usb").read_text())
        except OSError:  # not found on the Orin yet
            return
        if usb == self.usb:
            return
        self.usb = usb
        for view in self.cameras.views:
            # Moved to another USB port (rewired as usb_advice says, a reset keeps the port): out of stuck, its ladder
            # from the start.
            path = usb.get(VENDOR[view], {}).get("path")
            if view in self.stuck and path and self.paths.get(view, path) != path:
                self.start_over(view)
                self.event("CameraBack", "%s is no longer stuck: it moved to USB %s (was %s)" % (view, path, self.paths[view]), "Normal")
            if path:
                self.paths[view] = path
        paths = {view: usb[VENDOR[view]]["path"] for view in self.cameras.views if VENDOR[view] in usb}
        hubs = defaultdict(list)  # the cameras on each hub (a camera on a root port: on that port)
        for view, path in paths.items():
            hubs[path.rsplit(".", 1)[0]].append(view)
        advice = {}
        for view, path in paths.items():
            root, own = path.split(".")[0], hubs[path.rsplit(".", 1)[0]]
            others = [other for hub, views in hubs.items() if hub.split(".")[0] == root and views is not own
                      and len(views) >= len(own) for other in views]
            if others:
                advice[view] = "与 %s 共用 USB 根口 %s（经 %d 级 hub），建议改接独立根口" % ("、".join(others), root, path.count("."))
        for view in self.cameras.views:
            self.records[view].update(usb=usb.get(VENDOR[view]), usb_advice=advice.get(view))

    def start_over(self, view):
        """``view``'s ladder from its start, out of stuck."""
        self.ladder.pop(view, None)
        self.reset_at.pop(view, None)
        self.stuck.discard(view)
        self.records[view].update(step=0, attempts=0, next_attempt_at=None, diagnosis=None)

    def book(self, views, restarted, result, now):
        """A driver restart for ``views``: an attempt each, and their pauses doubled. The vendor's service away is no
        attempt (a USB reset would not bring it back), and no backoff: tried again in a minute."""
        for view in views:
            place = self.ladder[view]
            place.update(step=1, attempts=place["attempts"] + restarted,
                         next_at=now + (place["pause"] if restarted else RESTART_SECONDS[0]))
            if restarted:
                place["pause"] = min(place["pause"] * 2, RESTART_SECONDS[1])
        self.mark(views, now, last_action="driver_restart", last_result=result)

    def give_up(self, reasons, now):
        """The cameras ``reasons`` names stuck, each for its reason, and their facts asked of the orin container."""
        self.retried_at = now
        for view, reason in reasons.items():
            self.stuck.add(view)
            self.records[view].update(step=3, attempts=self.ladder[view]["attempts"], next_attempt_at=None,
                                      last_action="stuck", last_result=reason, diagnosis=None)
        self.diagnosing = self.ask("diagnose", (self.diagnosing[2] if self.diagnosing else []) + list(reasons), now)

    def restart_drivers(self, views, why):
        """(restarted, what came of it); an Event either way."""
        try:
            self.restart()
        except Exception as error:  # the vendor's monitor is away or refused: said, and tried again
            self.event("CameraDriverRestart", "the camera driver was not restarted for %s (%s): %s" % (", ".join(views), why, error), "Warning")
            return False, "vendor service failed: %s" % error
        self.event("CameraDriverRestart", "the camera driver restarted for %s (%s)" % (", ".join(views), why), "Normal")
        return True, "restarted"

    def tick(self, now):
        """One look, once a second: at most one step."""
        self.locate()
        if self.diagnosing:
            result = self.answer("diagnose", self.diagnosing, now)
            if result is not None:  # the orin container wrote the cameras' USB devices before its answer
                for view in self.diagnosing[2]:
                    diagnosis = result["cameras"][VENDOR[view]] if result else "orin 容器没有回答"
                    if view in self.stuck:
                        self.records[view]["diagnosis"] = diagnosis
                    self.event("CameraStuck", "%s: %s；%s" % (view, self.records[view]["last_result"], diagnosis), "Warning")
                self.diagnosing = None
        states = self.cameras.states(now)
        for view, state in states.items():
            down = state in ("stale", "absent")
            if down != self.since[view][0]:
                self.since[view] = (down, now)
            if not down and view in self.ladder and now - self.since[view][1] >= HEALTHY_SECONDS:
                # Sent for 5 minutes: its ladder starts over, a stuck camera with it. One that comes back for a moment
                # and stalls again goes on up the ladder, or stays stuck.
                if view in self.stuck:
                    self.event("CameraBack", "%s is no longer stuck: it has sent for %d s" % (view, HEALTHY_SECONDS), "Normal")
                self.start_over(view)
            elif state == "ok" and view not in self.ladder:  # one on its ladder says where it is until it starts over
                self.records[view].update(step=0, attempts=0, next_attempt_at=None)
        down = [view for view, (gone, _) in self.since.items() if gone and view not in self.stuck]
        if self.request and not any(self.since[view][0] for view in self.request[2]):  # back before the Orin reset them
            (self.directory / "reset-request").unlink(missing_ok=True)
            self.request = None
        if now - self.started < GRACE_SECONDS:
            return
        # Down for 30 s, each camera by its own stall: a driver restart's blank of another's is no step.
        active = [view for view in down if now - self.since[view][1] >= SETTLE_SECONDS]
        restart = [view for view in active if self.ladder[view]["attempts"] < RESTARTS]
        if self.request:
            result = self.answer("reset", self.request, now)
            if result is None:
                return
            views, self.request = self.request[2], None
            detail = result.get("detail", "the orin container did not answer")
            reset = [view for view in views if VENDOR[view] in result.get("reset", ())]
            missing = [view for view in views if VENDOR[view] in result.get("missing", ())]
            self.event("CameraUsbReset", "USB reset of %s: %s" % (", ".join(views), detail), "Normal" if result.get("ok") else "Warning")
            # Not run (ssh, the Orin or the orin container away): asked again after its pause, nothing blanked.
            unrun = [view for view in views if view not in reset + missing]
            for view in unrun:
                place = self.ladder[view]
                place["next_at"] = now + place["pause"]
                place["pause"] = min(place["pause"] * 2, RESTART_SECONDS[1])
            self.mark(unrun, now, last_action="usb_reset", last_result=detail)
            if missing:
                self.give_up(dict.fromkeys(missing, NO_DEVICE), now)
            if reset:
                # The driver once more, which counts for every other camera down that has restarts to go.
                others = [view for view in restart if view not in views]
                restarted, said = self.restart_drivers(reset + others, "after the USB reset" + (
                    " of %s" % ", ".join(reset) if others else ""))
                # From the restart's end: frames while the driver restarts are not the reset's.
                self.reset_at.update(dict.fromkeys(reset, self.cameras.clock()))
                for view in reset:
                    self.ladder[view].update(reset=True, next_at=now + RESTART_SECONDS[0])
                self.mark(reset, now, last_action="usb_reset", last_result="%s; driver %s" % (detail, said))
                self.book(others, restarted, said, now)
            return
        # Stuck and still without frames: the driver once an hour, only while nobody uses the robot.
        dark = [view for view in self.stuck if states[view] in ("stale", "absent")]
        if dark and now - self.retried_at >= STUCK_RETRY_SECONDS and not self.busy():
            self.retried_at = now
            self.restart_drivers(sorted(dark, key=VIEWS.index), "stuck, the robot not in use")
            return
        if not down:
            slow = [view for view, state in states.items() if state == "slow" and view not in self.stuck]
            if slow and (self.slow_at is None or now - self.slow_at >= SLOW_RESTART_SECONDS):
                self.slow_at = now
                _, result = self.restart_drivers(slow, "under %d frames a second" % SLOW_FPS)
                for view in slow:
                    self.records[view].update(step=1, attempts=1, last_action="driver_restart", last_result=result,
                                              next_attempt_at=round(self.wall() + SLOW_RESTART_SECONDS))
            return
        # Back after its USB reset and stalled again within 5 minutes: the reset does not hold, nor would a restart.
        held = {view: self.cameras.frames[view]["received"] - self.reset_at[view] for view in active if view in self.reset_at
                and self.cameras.frames[view]["received"] is not None}
        suspect = {view: SUSPECT % seconds for view, seconds in held.items() if 0 < seconds < HEALTHY_SECONDS}
        if suspect:
            self.give_up(suspect, now)
            return
        due = [view for view in active if now >= self.ladder[view]["next_at"]]
        unreset = [view for view in due if not self.ladder[view]["reset"]]
        if set(due) & set(restart):
            # One restart is every camera's that still has restarts to go.
            self.book(restart, *self.restart_drivers(restart, "no frames"), now)
        elif unreset:
            for view in unreset:
                self.ladder[view]["step"] = 2
            self.request = self.ask("reset", unreset, now)
            self.mark(unreset, now, timed=False, last_action="usb_reset", last_result="asked")
        elif due:
            self.give_up(dict.fromkeys(due, STUCK), now)


def report(cameras, recovery, now, wall):
    """What GET /cameras answers and the Node's annotation holds."""
    states = cameras.states(now)
    down = [view for view, state in states.items() if state in ("stale", "absent")]
    overall = ("stuck" if any(view in recovery.stuck for view in down) else "recovering" if down else
               "degraded" if "slow" in states.values() else "ok")
    return dict(state=overall, updated_at=round(wall(), 1), cameras={
        view: dict(recovery.records[view], state=states[view], fps=cameras.fps(view), age_seconds=cameras.age(view, now))
        for view in cameras.views})


def publish(said, published, patch):
    """``said`` (a report) written to the Node through ``patch`` when it differs from what was last written in more
    than the rates and ages; what is written now."""
    key = (said["state"], [{name: value for name, value in camera.items() if name not in ("fps", "age_seconds")}
                           for camera in said["cameras"].values()])
    if key == published:
        return published
    try:
        patch(dict(labels={STATE: said["state"]}, annotations={ANNOTATION: json.dumps(said, ensure_ascii=False)}))
    except OSError as error:
        print("the Node is not written: %s" % error, flush=True)
        return published
    print("%s cameras %s (%s)" % (time.strftime("%F %T"), said["state"], ", ".join(
        "%s %s" % (view, camera["state"]) for view, camera in said["cameras"].items())), flush=True)
    return key


def handler(cameras, recovery, wall=time.time, body_state=None):
    open_streams = threading.Semaphore(STREAMS)

    class Handler(BaseHTTPRequestHandler):
        def setup(self):
            super().setup()
            self.connection.settimeout(10)  # a viewer that stopped reading is let go

        def log_message(self, *arguments):
            pass

        def answer(self, status, kind, body, **headers):
            self.send_response(status)
            self.send_header("Content-Type", kind)
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(body)))
            for name, value in headers.items():
                self.send_header(name.replace("_", "-"), str(value))
            self.end_headers()
            self.wfile.write(body)

        def refuse(self, status, detail):
            self.answer(status, "application/json", json.dumps(dict(detail=detail)).encode())

        def do_GET(self):
            url = urlparse(self.path)
            if url.path == "/body-state":
                snapshot = body_state() if body_state else None
                if snapshot is None:
                    return self.refuse(404, "Body state is unavailable")
                return self.answer(200, "application/json", json.dumps(snapshot, allow_nan=False).encode())
            if url.path == "/cameras":
                received_ms = wall() * 1000
                said = report(cameras, recovery, cameras.clock(), wall)
                # Only the HTTP report carries clock samples. The Node annotation is
                # still the state report, without per-request timestamps.
                said["clock"] = dict(received_ms=received_ms, sent_ms=wall() * 1000)
                return self.answer(200, "application/json", json.dumps(said, ensure_ascii=False).encode())
            match = re.fullmatch(r"/cameras/(%s|wall)(\.jpg|/stream)" % "|".join(VIEWS), url.path)
            if not match or match[1] not in cameras.views + ("wall",):
                return self.refuse(404, "Not found")
            view = match[1]
            if match[2] == ".jpg" and view == "wall":
                return self.answer(200, "image/jpeg", cameras.wall())
            if view != "wall":
                jpeg, age = cameras.jpeg(view)
                if jpeg is None:
                    return self.refuse(503, "no frame of %s yet" % view)
                if match[2] == ".jpg":
                    return self.answer(200, "image/jpeg", jpeg, X_Age_Seconds=age)
            most, usual = (10, 5) if view == "wall" else (30, 10)
            rate = parse_qs(url.query).get("fps", [str(usual)])[0]
            if not rate.isdigit() or not 1 <= int(rate) <= most:
                return self.refuse(400, "fps is 1 to %d" % most)
            if not open_streams.acquire(blocking=False):
                return self.refuse(503, "%d streams are open already" % STREAMS)
            try:
                self.stream(view, int(rate))
            except OSError:
                pass  # the viewer left
            finally:
                open_streams.release()

        def stream(self, view, rate):
            self.send_response(200)
            self.send_header("Content-Type", "multipart/x-mixed-replace; boundary=" + BOUNDARY)
            self.send_header("Cache-Control", "no-store")
            self.send_header("X-Accel-Buffering", "no")  # the console's nginx passes each part on as it comes
            self.end_headers()
            sequence = None
            while True:
                began = time.monotonic()
                if view == "wall":
                    jpeg, aged = cameras.wall(), ""
                else:
                    jpeg, age, sequence, captured = cameras.next(view, sequence)
                    aged = "X-Age-Seconds: %s\r\n" % age
                    if captured is not None:
                        aged += "X-Camera-Capture-Ms: %.3f\r\n" % captured
                self.wfile.write(("--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n%s\r\n"
                                  % (BOUNDARY, len(jpeg), aged)).encode() + jpeg + b"\r\n")
                self.wfile.flush()
                time.sleep(max(0.0, 1 / rate - (time.monotonic() - began)))

    return Handler


def api(method, path, body=None):
    """The API server's reply with the Pod's service account."""
    request = Request(
        "https://%s:%s%s" % (os.environ["KUBERNETES_SERVICE_HOST"], os.environ["KUBERNETES_SERVICE_PORT"], path),
        data=None if body is None else json.dumps(body).encode(), method=method,
        headers={"Authorization": "Bearer " + open(ACCOUNT + "/token").read(),
                 "Content-Type": "application/merge-patch+json" if method == "PATCH" else "application/json"})
    with urlopen(request, context=ssl.create_default_context(cafile=ACCOUNT + "/ca.crt"), timeout=10) as reply:
        return json.load(reply)


def follow(cameras, recovery, node):
    """Every second: the rates, one step of the ladder, and the state on the Node when it changed."""
    published = None
    while True:
        now = cameras.clock()
        cameras.sample(now)
        recovery.tick(now)
        published = publish(report(cameras, recovery, now, time.time), published,
                            lambda patch: api("PATCH", "/api/v1/nodes/" + node, dict(metadata=patch)))
        time.sleep(1.0)


def watch_master(uri):
    """Once a second, the process ID of the ROS master this process subscribed at; when it changes (the master
    restarted), the subscriptions are gone with the old master and this ends the process (vital)."""
    first = None
    while True:
        try:
            pid = ServerProxy(uri).getPid("/s1u_cameras")[2]
        except OSError:  # the master is down
            pid = first
        first = first or pid
        if pid != first:
            raise RuntimeError("the ROS master at %s restarted (process %s, was %s)" % (uri, pid, first))
        time.sleep(1.0)


def main():
    lines = (line.split("=", 1) for line in open(os.environ["S1U_NODE_PROFILE"]).read().splitlines()
             if "=" in line and line[0] != "#")
    profile = {key: value.strip('"') for key, value in lines}

    def cpus(text):
        return {cpu for part in text.split(",") for cpu in range(int(part.split("-")[0]), int(part.split("-")[-1]) + 1)}

    # On the device layer's CPUs, off the SDK command thread's core.
    os.sched_setaffinity(0, cpus(profile["S1U_DEVICE_CPUS"]) - cpus(profile["S1U_SDK_CPUS"]))
    os.environ.update(ROS_MASTER_URI="http://%s:11311" % profile["S1U_ROS_IP_ADDRESS"], ROS_IP=profile["S1U_ROS_IP_ADDRESS"])
    import rospy
    from std_srvs.srv import SetBool

    node = os.environ["NODE_NAME"]
    metadata = api("GET", "/api/v1/nodes/" + node)["metadata"]
    body = metadata["labels"]["rlark.io/robot-body"]
    cameras = Cameras(json.loads(Path(os.environ["S1U_BODIES"]).read_text())[body])

    def call(value):
        """The vendor's service called on a thread of its own: one that hangs does not stop the ladder."""
        rospy.wait_for_service(SERVICE, timeout=5.0)
        caller = ThreadPoolExecutor(1)
        try:
            return caller.submit(rospy.ServiceProxy(SERVICE, SetBool), value).result(CALL_SECONDS)
        except Unanswered:
            raise RuntimeError("no answer in %d s" % CALL_SECONDS) from None
        finally:
            caller.shutdown(wait=False)

    def restart():
        call(False)  # a driver that does not run (after a boot) has nothing to stop: only the start must succeed
        time.sleep(2.0)
        reply = call(True)
        if not reply.success:
            raise RuntimeError(reply.message)

    def event(reason, message, kind):
        print("%s %s: %s" % (time.strftime("%F %T"), reason, message), flush=True)
        at = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        try:
            api("POST", "/api/v1/namespaces/default/events", dict(
                metadata=dict(generateName="s1u-cameras-"), type=kind, reason=reason, message="%s: %s" % (node, message),
                involvedObject=dict(kind="Node", name=node), source=dict(component="s1u-cameras"),
                firstTimestamp=at, lastTimestamp=at, count=1))
        except OSError as error:
            print("the Event is not written: %s" % error, flush=True)

    recovery = Recovery(cameras, restart, os.environ["S1U_CAMERA_DIRECTORY"], event)
    # The port first: with the robot's master down the cameras are absent, the service is not missing.
    observer = None
    server = ThreadingHTTPServer((os.environ["S1U_NODE_ADDRESS"], PORT),
                                 handler(cameras, recovery, body_state=lambda: observer() if observer else None))
    server.daemon_threads = True
    threading.Thread(target=server.serve_forever, daemon=True).start()
    vital(follow, cameras, recovery, node)
    vital(watch_master, os.environ["ROS_MASTER_URI"])
    print("Cameras of %s (%s, %s) on %s:%d" % (node, body, ", ".join(cameras.views), os.environ["S1U_NODE_ADDRESS"], PORT), flush=True)
    rospy.init_node("s1u_cameras", disable_signals=True)  # waits for the master
    for view in cameras.views:
        rospy.Subscriber(TOPIC % VENDOR[view], rospy.AnyMsg, lambda message, view=view: cameras.receive(view, message._buff),
                         queue_size=1, buff_size=16 * 1024 * 1024)
    try:
        capabilities = json.loads(Path("/s1u/device/capabilities.json").read_text())
        observer, body_subscriptions = body_observer.subscribe(rospy, capabilities, body, metadata["uid"])
    except (OSError, ValueError, KeyError) as error:
        print("Read-only body state unavailable: %s" % error, flush=True)
    threading.Event().wait()


if __name__ == "__main__":
    main()
