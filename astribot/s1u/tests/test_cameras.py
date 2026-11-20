"""The robot's camera service (astribot/s1u/device/cameras.py): a body's views, what each camera is, the ladder that
brings cameras back, what the Node is told, the HTTP it serves."""
import http.client
import json
import struct
import sys
import threading
from http.server import ThreadingHTTPServer
from xmlrpc.server import SimpleXMLRPCServer
from pathlib import Path

import cv2
import numpy
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "device"))
import cameras  # noqa: E402

BODIES = json.loads((Path(__file__).resolve().parents[1] / "tuner/bodies.json").read_text())


def message(colour=(200, 30, 30), size=(640, 360)):
    """A serialized sensor_msgs/CompressedImage of one colour (RGB), as the vendor's compressed topic carries it."""
    jpeg = cv2.imencode(".jpg", numpy.full((size[1], size[0], 3), colour[::-1], numpy.uint8))[1].tobytes()
    return wrap(jpeg), jpeg


def wrap(jpeg):
    frame = b"head_rgbd_color_optical_frame"
    return (struct.pack("<3I", 7, 1795000000, 5) + struct.pack("<I", len(frame)) + frame + struct.pack("<I", 4) + b"jpeg"
            + struct.pack("<I", len(jpeg)) + jpeg)


class Robot:
    """A body's cameras under a clock of the test's, with the ladder's doings recorded."""

    def __init__(self, tmp_path, body="s1u"):
        self.now, self.restarts, self.events, self.fails = 0.0, [], [], None
        self.cameras = cameras.Cameras(BODIES[body], clock=lambda: self.now)
        self.recovery = cameras.Recovery(self.cameras, self.restart, tmp_path, lambda *event: self.events.append(event),
                                         wall=lambda: 1795000000.0 + self.now)
        self.sending = set(self.cameras.views)
        self.rate = dict.fromkeys(self.cameras.views, 30)

    def restart(self):
        if self.fails:
            raise RuntimeError(self.fails)
        self.restarts.append(self.now)

    def run(self, seconds):
        """``seconds`` of the service's loop, the cameras that send sending at their rates."""
        for _ in range(int(seconds)):
            self.now += 1.0
            for view in self.sending:
                for _ in range(self.rate[view]):
                    self.cameras.receive(view, b"frame")
            self.cameras.sample(self.now)
            self.recovery.tick(self.now)

    def said(self):
        return cameras.report(self.cameras, self.recovery, self.now, self.recovery.wall)


def test_the_views_are_four_in_the_walls_order_and_a_body_has_its_own():
    assert cameras.VIEWS == ("head", "torso", "wrist_left", "wrist_right")
    assert cameras.VENDOR == {"head": "head_rgbd", "torso": "torso_rgbd", "wrist_left": "left_wrist_rgbd",
                              "wrist_right": "right_wrist_rgbd"}
    assert cameras.Cameras(BODIES["s1u"]).views == cameras.VIEWS
    assert cameras.Cameras(BODIES["s1u_observer"]).views == ("head", "torso")
    assert cameras.Cameras(["wrist_left", "head"]).views == ("head", "wrist_left")  # the wall's order, however listed
    assert cameras.TOPIC % cameras.VENDOR["torso"] == "/astribot_camera/torso_rgbd/color_compress/compressed"
    raw, jpeg = message()
    assert cameras.jpeg_of(raw) == jpeg  # the camera's own JPEG, taken out of the message without decoding


def test_a_camera_is_ok_slow_stale_or_absent_by_its_topic_alone(tmp_path):
    robot = Robot(tmp_path)
    robot.sending = {"head", "torso", "wrist_left"}
    robot.rate["torso"] = 6
    robot.run(10)
    said = robot.said()
    assert said["state"] == "recovering" and set(said) == {"state", "cameras", "updated_at"}
    assert {view: camera["state"] for view, camera in said["cameras"].items()} == {
        "head": "ok", "torso": "ok", "wrist_left": "ok", "wrist_right": "absent"}  # under 10 a second, not yet for 30 s
    assert said["cameras"]["head"] == dict(state="ok", fps=30.0, age_seconds=0.0, step=0, attempts=0, next_attempt_at=None,
                                           last_action=None, last_result=None)
    assert said["cameras"]["torso"]["fps"] == 6.0 and said["cameras"]["wrist_right"]["age_seconds"] is None
    robot.run(25)
    assert robot.said()["cameras"]["torso"]["state"] == "slow"
    robot.sending = {"head", "torso"}
    robot.run(5)
    assert robot.said()["cameras"]["wrist_left"]["state"] == "ok"  # 5 s without a frame is not yet stale
    robot.run(1)
    left = robot.said()["cameras"]["wrist_left"]
    assert left["state"] == "stale" and left["age_seconds"] == 6.0
    # Back again: stale no more at once, and not slow for the seconds its rate takes to show.
    robot.sending = {"head", "torso", "wrist_left"}
    robot.run(1)
    assert robot.said()["cameras"]["wrist_left"]["state"] == "ok"
    # A body's slow cameras alone are degraded, not recovering.
    observer = Robot(tmp_path, "s1u_observer")
    observer.rate["torso"] = 6
    observer.run(40)
    assert observer.said()["state"] == "degraded" and set(observer.said()["cameras"]) == {"head", "torso"}


def test_the_ladder_restarts_the_driver_resets_the_usb_and_says_stuck(tmp_path):
    robot = Robot(tmp_path)
    robot.sending = {"torso", "wrist_left", "wrist_right"}  # the head camera wedged (s1u_01 11-13)
    robot.run(89)
    assert not robot.restarts and not robot.events and robot.said()["cameras"]["head"]["step"] == 0  # the first 90 s
    robot.run(1)
    assert robot.restarts == [90.0]
    head = robot.said()["cameras"]["head"]
    assert head["step"] == 1 and head["attempts"] == 1 and head["last_action"] == "driver_restart" and head["last_result"] == "restarted"
    assert head["next_attempt_at"] == 1795000000 + 150 and robot.said()["cameras"]["torso"]["step"] == 0
    assert robot.events == [("CameraDriverRestart", "the camera driver restarted for head (no frames)", "Normal")]
    # 60 s, then 120 s, between the three restarts; 240 s after the third the USB reset is asked for.
    robot.run(600)
    assert robot.restarts == [90.0, 150.0, 270.0]
    request = json.loads((tmp_path / "reset-request").read_text())
    assert request["cameras"] == ["head_rgbd"] and len(request["id"]) == 32  # the vendor's name, as the Orin knows it
    head = robot.said()["cameras"]["head"]
    assert head["step"] == 2 and head["attempts"] == 3 and head["last_action"] == "usb_reset" and head["last_result"] == "asked"
    assert robot.said()["state"] == "recovering"
    # A result of another request is not this one's.
    (tmp_path / "reset-result").write_text(json.dumps(dict(id="0" * 32, ok=True, detail="reset ok old")))
    robot.run(10)
    assert len(robot.restarts) == 3
    (tmp_path / "reset-result").write_text(json.dumps(dict(id=request["id"], ok=True, detail="reset ok head_rgbd bus 002 dev 008")))
    robot.run(1)
    assert len(robot.restarts) == 4  # the driver once more after the reset
    assert robot.events[-2:] == [
        ("CameraUsbReset", "USB reset of head: reset ok head_rgbd bus 002 dev 008", "Normal"),
        ("CameraDriverRestart", "the camera driver restarted for head (after the USB reset)", "Normal")]
    assert robot.said()["cameras"]["head"]["last_result"] == "reset ok head_rgbd bus 002 dev 008; driver restarted"
    robot.run(59)
    assert robot.said()["state"] == "recovering"
    robot.run(1)
    said = robot.said()
    assert said["state"] == "stuck" and said["cameras"]["head"]["step"] == 3
    assert said["cameras"]["head"]["last_result"] == "相机需要断电或 USB 复位" == cameras.STUCK
    assert robot.events[-1] == ("CameraStuck", "head: 相机需要断电或 USB 复位", "Warning")
    stuck_at = robot.now
    # Nothing more for an hour, then the whole ladder again, with a request of its own.
    robot.run(3599)
    assert len(robot.restarts) == 4 and len(robot.events) == 6
    robot.run(1)
    assert robot.restarts[-1] == stuck_at + 3600 and robot.said()["cameras"]["head"]["step"] == 1 and robot.said()["state"] == "recovering"
    # The frames are back for a moment: said ok, and what was last done stays said.
    restarted = robot.restarts[-1]
    robot.sending.add("head")
    robot.run(2)
    head = robot.said()["cameras"]["head"]
    assert robot.said()["state"] == "ok" and head["step"] == 0 and head["attempts"] == 0 and head["next_attempt_at"] is None
    assert head["last_action"] == "driver_restart"
    # Stalled again (the Femto Bolt of 11-13 sent a burst now and then): the ladder goes on where it was, 60 s after
    # the restart, not from its start.
    robot.sending.discard("head")
    robot.run(restarted + 59 - robot.now)
    assert robot.restarts[-1] == restarted
    robot.run(1)
    assert robot.restarts[-1] == restarted + 60 and robot.said()["cameras"]["head"]["attempts"] == 2
    # Five minutes of frames start it over: stalled again, the first restart 30 s after the frames stopped.
    robot.sending.add("head")
    robot.run(301)
    assert robot.recovery.step == 0
    robot.sending.discard("head")
    stopped = robot.now
    robot.run(35)
    assert robot.restarts[-1] == restarted + 60
    robot.run(1)
    assert robot.restarts[-1] == stopped + 36 and robot.said()["cameras"]["head"]["attempts"] == 1


def test_the_vendors_service_away_is_no_attempt_and_the_orins_silence_ends_the_wait(tmp_path):
    robot = Robot(tmp_path, "s1u_observer")
    robot.sending = set()  # after a boot: the driver does not run yet
    robot.fails = "service [/astribot_system_monitor/camera_control_command] unavailable"
    robot.run(1000)
    head = robot.said()["cameras"]["head"]
    assert head["state"] == "absent" and head["step"] == 1 and head["attempts"] == 0
    assert head["last_result"] == "vendor service failed: " + robot.fails and not (tmp_path / "reset-request").exists()
    assert [event[2] for event in robot.events] == ["Warning"] * 16  # every minute from 90 s: no backoff
    # The monitor is up: the driver starts on the restart's True within a minute, and the cameras send.
    robot.fails = None
    robot.run(60)
    assert len(robot.restarts) == 1
    robot.sending = {"head", "torso"}
    robot.run(301)
    assert robot.said()["state"] == "ok" and robot.recovery.step == 0
    # An orin container that does not answer: after 300 s the ladder goes on to the restart and to stuck.
    robot.sending = {"head"}
    robot.run(6 + 30 + 60 + 120 + 240)
    assert robot.said()["cameras"]["torso"]["step"] == 2
    robot.run(300)
    assert robot.said()["cameras"]["torso"]["last_result"] == "the orin container did not answer; driver restarted"
    robot.run(60)
    assert robot.said()["state"] == "stuck"


def test_a_slow_camera_gets_the_driver_restart_only_and_once_an_hour(tmp_path):
    robot = Robot(tmp_path)
    robot.rate["wrist_right"] = 5
    robot.run(89)
    assert not robot.restarts
    robot.run(1)
    assert robot.restarts == [90.0] and robot.said()["state"] == "degraded"
    right = robot.said()["cameras"]["wrist_right"]
    assert right["state"] == "slow" and right["step"] == 1 and right["last_action"] == "driver_restart"
    assert right["next_attempt_at"] == 1795000000 + 90 + 3600
    assert robot.events == [("CameraDriverRestart", "the camera driver restarted for wrist_right (under 10 frames a second)", "Normal")]
    # The restart blanks every camera for about 25 s: no camera failure, nothing more done.
    robot.sending = set()
    robot.run(25)
    assert robot.said()["state"] == "recovering"
    robot.sending = set(robot.cameras.views)
    robot.run(3574)
    assert len(robot.restarts) == 1 and not (tmp_path / "reset-request").exists()
    robot.run(1)
    assert len(robot.restarts) == 2


def test_a_reset_the_frames_came_back_before_is_withdrawn(tmp_path):
    (tmp_path / "reset-request").write_text(json.dumps(dict(id="0" * 32, cameras=["head_rgbd"])))
    robot = Robot(tmp_path, "s1u_observer")
    assert not (tmp_path / "reset-request").exists()  # an earlier run's, which nobody waits for
    robot.sending = {"torso"}
    robot.run(510)
    asked = json.loads((tmp_path / "reset-request").read_text())["id"]
    robot.sending.add("head")
    robot.run(1)
    assert not (tmp_path / "reset-request").exists() and "CameraUsbReset" not in [event[0] for event in robot.events]
    # Stalled again: asked again, 30 s after the frames stopped.
    robot.sending.discard("head")
    robot.run(35)
    assert not (tmp_path / "reset-request").exists()
    robot.run(1)
    assert json.loads((tmp_path / "reset-request").read_text())["id"] != asked


def test_a_restarted_ros_master_ends_the_process():
    """rospy does not subscribe again at a new master: the process ends (vital) and kubelet starts it afresh."""
    pids = [101, 101, 202]
    master = SimpleXMLRPCServer(("127.0.0.1", 0), logRequests=False)
    master.register_function(lambda caller: [1, "", pids.pop(0) if len(pids) > 1 else pids[0]], "getPid")
    threading.Thread(target=master.serve_forever, daemon=True).start()
    with pytest.raises(RuntimeError, match="restarted .process 202, was 101"):
        cameras.watch_master("http://127.0.0.1:%d" % master.server_address[1])
    master.shutdown()
    master.server_close()


def test_the_node_is_written_when_the_state_changes_and_not_for_rates_and_ages(tmp_path):
    robot = Robot(tmp_path, "s1u_observer")
    patches = []
    robot.run(3)
    published = cameras.publish(robot.said(), None, patches.append)
    assert patches[0]["labels"] == {"rlark.io/camera-state": "ok"}
    annotation = json.loads(patches[0]["annotations"]["rlark.io/cameras"])
    assert annotation == robot.said() and set(annotation["cameras"]) == {"head", "torso"}
    robot.rate["head"] = 29
    robot.run(3)
    assert cameras.publish(robot.said(), published, patches.append) == published and len(patches) == 1
    robot.sending = {"torso"}
    robot.run(6)
    published = cameras.publish(robot.said(), published, patches.append)
    assert patches[1]["labels"] == {"rlark.io/camera-state": "recovering"}
    assert json.loads(patches[1]["annotations"]["rlark.io/cameras"])["cameras"]["head"]["state"] == "stale"

    def away(patch):
        raise OSError("the API server is away")

    robot.sending = {"head", "torso"}
    robot.run(2)
    assert cameras.publish(robot.said(), published, away) == published  # written at the next look
    assert cameras.publish(robot.said(), published, patches.append) != published and patches[2]["labels"] == {"rlark.io/camera-state": "ok"}


@pytest.fixture
def served(tmp_path):
    cameras_ = cameras.Cameras(BODIES["s1u_observer"])
    recovery = cameras.Recovery(cameras_, lambda: None, tmp_path, lambda *event: None)
    server = ThreadingHTTPServer(("127.0.0.1", 0), cameras.handler(cameras_, recovery))
    server.daemon_threads = True
    threading.Thread(target=server.serve_forever, daemon=True).start()

    def get(path):
        connection = http.client.HTTPConnection("127.0.0.1", server.server_address[1], timeout=5)
        connection.request("GET", path)
        return connection.getresponse()

    yield cameras_, get
    server.shutdown()
    server.server_close()


def image(jpeg):
    return cv2.imdecode(numpy.frombuffer(jpeg, numpy.uint8), cv2.IMREAD_COLOR)


def part(response):
    """One part of a multipart stream: (its headers, its JPEG)."""
    assert response.fp.readline() == b"--s1ucamera\r\n"
    headers = {}
    for line in iter(response.fp.readline, b"\r\n"):
        name, value = line.decode().strip().split(": ")
        headers[name] = value
    jpeg = response.fp.read(int(headers["Content-Length"]))
    assert response.fp.readline() == b"\r\n"
    return headers, jpeg


def test_the_port_serves_the_state_each_views_jpeg_its_stream_and_the_wall(served):
    cameras_, get = served
    said = json.loads(get("/cameras").read())
    assert said["state"] == "recovering" and said["cameras"]["torso"]["state"] == "absent"
    assert get("/cameras/head.jpg").status == 503 and get("/cameras/head/stream").status == 503  # no frame yet
    assert get("/cameras/wrist_left.jpg").status == 404 and get("/cameras/wrist_left/stream").status == 404  # not this body's
    assert get("/cameras/stereo.jpg").status == 404 and get("/status").status == 404 and get("/cameras/head").status == 404
    raw, jpeg = message((200, 30, 30), (1280, 720))
    cameras_.receive("head", raw)
    response = get("/cameras/head.jpg")
    assert response.status == 200 and response.getheader("Content-Type") == "image/jpeg" and response.read() == jpeg
    assert float(response.getheader("X-Age-Seconds")) < 2 and response.getheader("Cache-Control") == "no-store"
    for rate in ("0", "31", "x"):
        assert get("/cameras/head/stream?fps=" + rate).status == 400
    assert get("/cameras/wall/stream?fps=11").status == 400
    # A view's stream is the camera's own JPEGs, each with its age; a camera that stopped sends its last one again.
    response = get("/cameras/head/stream?fps=30")
    assert response.getheader("Content-Type") == "multipart/x-mixed-replace; boundary=s1ucamera"
    assert response.getheader("X-Accel-Buffering") == "no"
    headers, first = part(response)
    assert first == jpeg and headers["Content-Type"] == "image/jpeg" and float(headers["X-Age-Seconds"]) < 2
    other, second_jpeg = message((30, 200, 30))
    cameras_.receive("head", other)
    assert part(response)[1] == second_jpeg
    assert part(response)[1] == second_jpeg  # after a second without a new frame
    response.close()
    # The wall: 256 x 144, head and torso above the wrists, a view without a fresh frame black.
    wall = image(get("/cameras/wall.jpg").read())
    assert wall.shape == (144, 256, 3)
    blue, green, red = wall[36, 64]
    assert green > 150 and red < 90 and blue < 90  # the head's newest frame
    for x, y in ((192, 36), (64, 108), (192, 108)):
        assert wall[y, x].max() < 20  # torso without a frame, and the wrists this body lacks
    cameras_.receive("torso", wrap(b"not a JPEG"))
    cameras_.composed = (None, None)  # made anew, not the one of a moment ago
    assert image(cameras_.wall()).shape == (144, 256, 3)  # a frame that does not decode is a black cell
    response = get("/cameras/wall/stream")
    headers, jpeg = part(response)
    assert image(jpeg).shape == (144, 256, 3) and "X-Age-Seconds" not in headers
    response.close()
