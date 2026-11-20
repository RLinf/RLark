"""The robot's cameras, a container of the Addon s1u-robot on every S1U robot x86 (video): the one reader of the
cameras for whoever looks at the robot, and their keeper. Python 3.8.

It subscribes each camera of the robot's body (its Node's label rlark.io/robot-body, bodies.json) from start to stop,
whether anyone watches or a Run uses the robot: the vendor's compressed topic
/astribot_camera/<camera>/color_compress/compressed, received raw (rospy.AnyMsg, no decode), of which it keeps the
newest JPEG, when it arrived and the rate.

Read-only HTTP on the node's address (S1U_NODE_ADDRESS, the site tunnel's), port 18891; RLark's Gateway reaches it
through the cluster's API server (the Pod's proxy) and decides who may look:
    GET /cameras                        {state, cameras: {view: {state, fps, age_seconds, step, attempts,
                                        next_attempt_at, last_action, last_result}}, updated_at}
    GET /cameras/<view>.jpg             the newest JPEG (X-Age-Seconds); 404 a view the body lacks, 503 no frame yet
    GET /cameras/<view>/stream?fps=N    multipart/x-mixed-replace of the camera's own JPEGs, N 1..30 (10); 404, 503 alike
    GET /cameras/wall.jpg               the four views as one 256 x 144 image: head, torso / wrist_left, wrist_right,
    GET /cameras/wall/stream?fps=N      each 128 x 72, a view that is missing or stale black; N 1..10 (5)
Only the wall decodes (OpenCV, at a quarter of the size in libjpeg itself), and only while someone asks for it.

A camera is ok | slow (under 10 frames a second for 30 s) | stale (no frame for 5 s) | absent (none since this
started). Health is the topic alone: the vendor's /astribot_error_code/multi_error_code keeps only the last of the
camera driver's codes and is not read. Cameras that are stale or absent for 30 s (longer than the blank of a driver
restart, anyone's) are brought back by a ladder (Recovery), each step an Event on the Node:
    1. the vendor's camera driver restarted (/astribot_system_monitor/camera_control_command False, 2 s, True: all
       cameras blank for about 25 s), up to 3 times, 60 s apart doubling to 900 s. It is also how the driver starts
       after a boot: the vendor's monitor starts it on True. A slow camera gets this step only, once an hour.
    2. the stale cameras' USB devices reset on the Orin: a request to the orin container through the Pod's shared
       directory (S1U_CAMERA_DIRECTORY: reset-request {id, cameras: the vendor's names}, reset-result {id, ok, detail,
       finished_at}), then the driver restarted once more (s1u_01 11-13 22:40: the head camera stalled inside the
       Orbbec SDK and no driver restart brought it back). A request the frames came back before is withdrawn.
    3. stuck: 相机需要断电或 USB 复位. The ladder starts over after an hour.
The ladder starts over once every camera has sent for 5 minutes: a camera that comes back for a moment and stalls
again goes on up it. Nothing is done in the first 90 s. The state is on the Node, written when it changes: label
rlark.io/camera-state ok | recovering | degraded (slow cameras only) | stuck, annotation rlark.io/cameras the JSON of
GET /cameras. A restart of the robot's ROS master ends this process (rospy does not subscribe again at a new master):
kubelet starts it afresh.
"""

import json
import os
import re
import ssl
import struct
import threading
import time
import uuid
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from concurrent.futures import TimeoutError as Unanswered
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse
from urllib.request import Request, urlopen
from xmlrpc.client import ServerProxy

import cv2
import numpy
from power import vital

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
HEALTHY_SECONDS = 300.0  # the ladder starts over once every camera has sent this long
RESTART_SECONDS, RESTARTS = (60.0, 900.0), 3
CALL_SECONDS = 30.0  # the vendor's monitor answers a call within this, or the call failed
RESET_SECONDS = 300.0  # the orin container's answer is waited for this long
STUCK_SECONDS = 3600.0
STUCK = "相机需要断电或 USB 复位"

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


class Cameras:
    """The body's cameras' newest frames, and what each camera is."""

    def __init__(self, views, clock=time.monotonic):
        self.views = tuple(view for view in VIEWS if view in views)
        self.clock = clock
        self.lock = threading.Lock()
        self.framed = threading.Condition(self.lock)
        # samples: (time, frames so far) of the last five seconds, one a second; slow_since: since when under SLOW_FPS.
        self.frames = {view: dict(raw=None, received=None, sequence=0, samples=deque(maxlen=6), slow_since=None)
                       for view in self.views}
        self.composed = (None, None)  # the wall as last made: (when, JPEG)

    def receive(self, view, raw):
        with self.lock:
            frame = self.frames[view]
            frame.update(raw=raw, received=self.clock(), sequence=frame["sequence"] + 1)
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
        """(JPEG, age, sequence) of the newest frame after ``sequence``, of a camera that sent one; a camera that
        stopped gives its last frame again each second, so a viewer sees its age grow."""
        with self.lock:
            frame = self.frames[view]
            self.framed.wait_for(lambda: frame["sequence"] != sequence, timeout=1)
            return jpeg_of(frame["raw"]), self.age(view, self.clock()), frame["sequence"]

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
    """The ladder that brings stale and absent cameras back. ``restart`` restarts the vendor's camera driver (it
    raises when the vendor's service is away); ``event(reason, message, kind)`` tells the Node."""

    def __init__(self, cameras, restart, directory, event, wall=time.time):
        self.cameras, self.restart, self.event, self.wall = cameras, restart, event, wall
        self.request_file, self.result_file = Path(directory) / "reset-request", Path(directory) / "reset-result"
        self.started = cameras.clock()
        self.records = {view: dict(step=0, attempts=0, next_attempt_at=None, last_action=None, last_result=None)
                        for view in cameras.views}
        self.slow_at = None  # when the driver was last restarted for a slow camera
        self.down_since = self.healthy_since = None
        self.request_file.unlink(missing_ok=True)  # an earlier run's request, which nobody waits for
        self.rewind()

    def rewind(self):
        self.step, self.attempts, self.pause, self.next_at = 0, 0, RESTART_SECONDS[0], 0.0
        self.request = None  # the USB reset asked for and not answered yet: (id, when)
        self.reset_done = False
        self.stuck_since = None

    def mark(self, views, now, next_at=None, **fields):
        """What the ladder did for ``views``, as GET /cameras says it; ``next_at``: when it acts next."""
        for view in views:
            self.records[view].update(fields, step=self.step, attempts=self.attempts, next_attempt_at=(
                None if next_at is None else round(self.wall() + next_at - now)))

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
        states = self.cameras.states(now)
        down = [view for view, state in states.items() if state in ("stale", "absent")]
        for view, state in states.items():
            if state == "ok":
                self.records[view].update(step=0, attempts=0, next_attempt_at=None)
        if not down:
            self.down_since = None
            if self.request:  # back before the Orin reset them
                self.request_file.unlink(missing_ok=True)
                self.request = None
            if self.healthy_since is None:
                self.healthy_since = now
            if self.step and now - self.healthy_since >= HEALTHY_SECONDS:
                self.rewind()
                for record in self.records.values():
                    record.update(step=0, attempts=0, next_attempt_at=None)
            slow = [view for view, state in states.items() if state == "slow"]
            if slow and now - self.started >= GRACE_SECONDS and (self.slow_at is None or now - self.slow_at >= SLOW_RESTART_SECONDS):
                self.slow_at = now
                _, result = self.restart_drivers(slow, "under %d frames a second" % SLOW_FPS)
                for view in slow:
                    self.records[view].update(step=1, attempts=1, last_action="driver_restart", last_result=result,
                                              next_attempt_at=round(self.wall() + SLOW_RESTART_SECONDS))
            return
        self.healthy_since = None
        if self.down_since is None:
            self.down_since = now
        if now - self.started < GRACE_SECONDS or now - self.down_since < SETTLE_SECONDS:
            return
        if self.step == 3:
            if now - self.stuck_since < STUCK_SECONDS:
                return
            self.rewind()
        if self.request:
            identifier, asked = self.request
            try:
                result = json.loads(self.result_file.read_text())
            except (OSError, ValueError):  # none yet, or being written
                result = {}
            if result.get("id") != identifier and now - asked < RESET_SECONDS:
                return
            detail = result["detail"] if result.get("id") == identifier else "the orin container did not answer"
            self.event("CameraUsbReset", "USB reset of %s: %s" % (", ".join(down), detail), "Normal")
            _, restarted = self.restart_drivers(down, "after the USB reset")
            self.request, self.reset_done, self.next_at = None, True, now + RESTART_SECONDS[0]
            self.mark(down, now, self.next_at, last_action="usb_reset", last_result="%s; driver %s" % (detail, restarted))
        elif now < self.next_at:
            return
        elif self.attempts < RESTARTS:
            self.step = 1
            restarted, result = self.restart_drivers(down, "no frames")
            # The vendor's service away is no attempt (a USB reset would not bring it back), and no backoff: tried
            # again in a minute.
            self.attempts += restarted
            self.next_at = now + (self.pause if restarted else RESTART_SECONDS[0])
            if restarted:
                self.pause = min(self.pause * 2, RESTART_SECONDS[1])
            self.mark(down, now, self.next_at, last_action="driver_restart", last_result=result)
        elif not self.reset_done:
            self.step = 2
            self.request = (uuid.uuid4().hex, now)
            self.request_file.with_suffix(".new").write_text(json.dumps(dict(id=self.request[0], cameras=[VENDOR[view] for view in down])) + "\n")
            self.request_file.with_suffix(".new").replace(self.request_file)
            self.mark(down, now, last_action="usb_reset", last_result="asked")
        else:
            self.step, self.stuck_since = 3, now
            self.event("CameraStuck", "%s: %s" % (", ".join(down), STUCK), "Warning")
            self.mark(down, now, now + STUCK_SECONDS, last_action="stuck", last_result=STUCK)


def report(cameras, recovery, now, wall):
    """What GET /cameras answers and the Node's annotation holds."""
    states = cameras.states(now)
    down = [state for state in states.values() if state in ("stale", "absent")]
    overall = "stuck" if recovery.step == 3 and down else "recovering" if down else "degraded" if "slow" in states.values() else "ok"
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


def handler(cameras, recovery, wall=time.time):
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
            if url.path == "/cameras":
                said = report(cameras, recovery, cameras.clock(), wall)
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
                    jpeg, age, sequence = cameras.next(view, sequence)
                    aged = "X-Age-Seconds: %s\r\n" % age
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
    body = api("GET", "/api/v1/nodes/" + node)["metadata"]["labels"]["rlark.io/robot-body"]
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
    server = ThreadingHTTPServer((os.environ["S1U_NODE_ADDRESS"], PORT), handler(cameras, recovery))
    server.daemon_threads = True
    threading.Thread(target=server.serve_forever, daemon=True).start()
    vital(follow, cameras, recovery, node)
    vital(watch_master, os.environ["ROS_MASTER_URI"])
    print("Cameras of %s (%s, %s) on %s:%d" % (node, body, ", ".join(cameras.views), os.environ["S1U_NODE_ADDRESS"], PORT), flush=True)
    rospy.init_node("s1u_cameras", disable_signals=True)  # waits for the master
    for view in cameras.views:
        rospy.Subscriber(TOPIC % VENDOR[view], rospy.AnyMsg, lambda message, view=view: cameras.receive(view, message._buff),
                         queue_size=1, buff_size=16 * 1024 * 1024)
    threading.Event().wait()


if __name__ == "__main__":
    main()
