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
        self.now, self.restarts, self.events, self.fails, self.busy = 0.0, [], [], None, ""
        self.cameras = cameras.Cameras(BODIES[body], clock=lambda: self.now)
        self.recovery = cameras.Recovery(self.cameras, self.restart, tmp_path, lambda *event: self.events.append(event),
                                         wall=lambda: 1795000000.0 + self.now, busy=lambda: self.busy)
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
                                           last_action=None, last_result=None, diagnosis=None, usb=None, usb_advice=None)
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
    (tmp_path / "reset-result").write_text(json.dumps(dict(id=request["id"], ok=True, reset=["head_rgbd"], missing=[], detail="reset ok head_rgbd bus 002 dev 008")))
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
    assert said["state"] == "stuck" and said["cameras"]["head"]["step"] == 3 and said["cameras"]["head"]["next_attempt_at"] is None
    assert said["cameras"]["head"]["last_result"] == "相机需要断电或 USB 复位" == cameras.STUCK
    # What the Orin shows of it is asked for; the Event says both once the orin container answers.
    asked = json.loads((tmp_path / "diagnose-request").read_text())
    assert asked["cameras"] == ["head_rgbd"] and robot.events[-1][0] == "CameraDriverRestart"
    (tmp_path / "diagnose-result").write_text(json.dumps(dict(id=asked["id"], cameras={"head_rgbd": "厂商相机进程在跑"})))
    robot.run(1)
    assert robot.events[-1] == ("CameraStuck", "head: 相机需要断电或 USB 复位；厂商相机进程在跑", "Warning")
    assert robot.said()["cameras"]["head"]["diagnosis"] == "厂商相机进程在跑"
    assert not (tmp_path / "diagnose-request").exists() and not (tmp_path / "reset-request").exists()  # each done once
    # Another camera that stalls at once has a ladder of its own: a driver restart 30 s after, not the head's stuck.
    robot.sending.discard("wrist_left")
    stopped = robot.now
    robot.run(36)
    assert robot.restarts[-1] == stopped + 36 and robot.said()["cameras"]["wrist_left"]["attempts"] == 1
    assert robot.events[-1] == ("CameraDriverRestart", "the camera driver restarted for wrist_left (no frames)", "Normal")
    robot.sending.add("wrist_left")
    # While a Run uses the robot, no restart for the stuck head: it would blank the cameras that work.
    robot.busy = "station: r-1"
    events = len(robot.events)
    robot.run(7200)
    assert len(robot.restarts) == 5 and len(robot.events) == events and robot.said()["state"] == "stuck"
    # Not in use: its driver once an hour, from an hour after it was given up; it stays stuck meanwhile.
    robot.busy = ""
    robot.run(1)
    assert robot.restarts[-1] == robot.now and robot.events[-1] == (
        "CameraDriverRestart", "the camera driver restarted for head (stuck, the robot not in use)", "Normal")
    assert robot.said()["state"] == "stuck" and robot.said()["cameras"]["head"]["last_result"] == cameras.STUCK
    retried = robot.now
    # A moment of frames is not out of stuck: stalled again, stuck again at once, and nothing until the next hour.
    robot.sending.add("head")
    robot.run(2)
    assert robot.said()["state"] == "ok" and robot.said()["cameras"]["head"]["step"] == 3
    robot.sending.discard("head")
    robot.run(6)
    assert robot.said()["state"] == "stuck"
    robot.run(retried + 3599 - robot.now)
    assert len(robot.restarts) == 6
    robot.run(1)
    assert robot.restarts[-1] == retried + 3600
    events = len(robot.events)
    # Five minutes of frames (someone powered it off and on, reseated it): out of stuck, and a stall is the ladder's
    # again, from its start since the others have sent all along.
    robot.sending.add("head")
    robot.run(300)
    assert len(robot.events) == events
    robot.run(1)
    head = robot.said()["cameras"]["head"]
    assert robot.events[-1] == ("CameraBack", "head is no longer stuck: it has sent for 300 s", "Normal")
    assert head["step"] == 0 and head["diagnosis"] is None and head["last_action"] == "stuck"
    robot.sending.discard("head")
    stopped = robot.now
    robot.run(35)
    assert len(robot.restarts) == 7
    robot.run(1)
    assert robot.restarts[-1] == stopped + 36 and robot.said()["cameras"]["head"]["attempts"] == 1
    # The frames are back for a moment: said ok, and where it is on the ladder stays said (s1u-03 11-21: back 5 to
    # 70 s at a time, and a 0th step said while it went on up the ladder).
    restarted = robot.restarts[-1]
    robot.sending.add("head")
    robot.run(2)
    head = robot.said()["cameras"]["head"]
    assert robot.said()["state"] == "ok" and head["step"] == 1 and head["attempts"] == 1
    assert head["last_action"] == "driver_restart"
    # Stalled again (the Femto Bolt of 11-13 sent a burst now and then): the ladder goes on where it was, 60 s after
    # the restart, not from its start.
    robot.sending.discard("head")
    robot.run(restarted + 59 - robot.now)
    assert robot.restarts[-1] == restarted
    robot.run(1)
    assert robot.restarts[-1] == restarted + 60 and robot.said()["cameras"]["head"]["attempts"] == 2


def test_a_camera_that_stalls_again_soon_after_its_usb_reset_is_stuck_at_once_and_the_others_keep_the_ladder(tmp_path):
    """s1u-03 11-21: the head's colour stream stopped 25 s to 3 min after each USB reset, and every hour the ladder
    restarted the driver of every camera again."""
    robot = Robot(tmp_path)
    (tmp_path / "usb").write_text(json.dumps({"head_rgbd": dict(path="2-3.3.3", device=11, speed=5000, hubs=["0bda:0420", "0424:5806"])}))
    robot.sending = {"torso", "wrist_left", "wrist_right"}
    robot.run(510)
    request = json.loads((tmp_path / "reset-request").read_text())
    (tmp_path / "reset-result").write_text(json.dumps(dict(id=request["id"], ok=True, reset=["head_rgbd"], missing=[], detail="reset ok head_rgbd bus 002 dev 012")))
    robot.run(1)
    assert len(robot.restarts) == 4
    robot.sending.add("head")  # back 1 s after the reset, for 100 s
    robot.run(100)
    robot.sending.discard("head")
    robot.run(35)
    assert robot.said()["state"] == "recovering"
    robot.run(1)
    head = robot.said()["cameras"]["head"]
    assert robot.said()["state"] == "stuck" and head["step"] == 3 and len(robot.restarts) == 4
    assert head["last_result"] == "疑似硬件：USB 复位后 100 秒又停，需要断电或现场检查接线"
    assert json.loads((tmp_path / "diagnose-request").read_text())["cameras"] == ["head_rgbd"]
    # Another camera that stalls has the whole ladder; the stuck one is no step of it.
    robot.sending.discard("wrist_left")
    stopped = robot.now
    robot.run(36)
    assert robot.restarts[-1] == stopped + 36 and robot.events[-1] == (
        "CameraDriverRestart", "the camera driver restarted for wrist_left (no frames)", "Normal")
    assert robot.said()["cameras"]["head"]["step"] == 3 and robot.said()["cameras"]["wrist_left"]["step"] == 1
    robot.sending.add("wrist_left")
    robot.run(1)
    assert robot.said()["state"] == "stuck"
    # A new device number on the same port (re-enumerated: every USB reset does it) is not out of stuck.
    (tmp_path / "usb").write_text(json.dumps({"head_rgbd": dict(path="2-3.3.3", device=12, speed=5000, hubs=["0bda:0420", "0424:5806"])}))
    robot.run(1)
    assert robot.said()["state"] == "stuck" and len(robot.restarts) == 5
    # The orin container silent: the Event says so after 300 s.
    robot.run(300 - 38)
    assert robot.events[-1] == ("CameraStuck", "head: " + head["last_result"] + "；orin 容器没有回答", "Warning")
    assert not (tmp_path / "diagnose-request").exists()  # nobody waits for it any more: not done late
    # Moved to a root port of its own, as usb_advice says: out of stuck, and its ladder from the start at once.
    (tmp_path / "usb").write_text(json.dumps({"head_rgbd": dict(path="2-2", device=3, speed=5000, hubs=[])}))
    robot.run(1)
    head = robot.said()["cameras"]["head"]
    assert robot.events[-2] == ("CameraBack", "head is no longer stuck: it moved to USB 2-2 (was 2-3.3.3)", "Normal")
    assert robot.events[-1] == ("CameraDriverRestart", "the camera driver restarted for head (no frames)", "Normal")
    assert robot.said()["state"] == "recovering" and head["step"] == 1 and head["attempts"] == 1 and len(robot.restarts) == 6


def test_a_driver_restart_for_another_camera_does_not_make_a_reset_camera_suspect(tmp_path):
    """Its blank of about 25 s is no stall of the camera's own; nor are frames while the driver restarts after the
    reset."""
    robot = Robot(tmp_path)
    robot.sending = {"torso", "wrist_left", "wrist_right"}
    robot.run(510)
    request = json.loads((tmp_path / "reset-request").read_text())
    (tmp_path / "reset-result").write_text(json.dumps(dict(id=request["id"], ok=True, reset=["head_rgbd"], missing=[], detail="reset ok head_rgbd bus 002 dev 012")))
    robot.run(1)
    robot.sending.add("head")
    robot.run(100)
    # The wrist stalls; its restarts blank every camera for 25 s.
    robot.sending.discard("wrist_left")
    robot.run(36)
    assert len(robot.restarts) == 5
    robot.sending.discard("head")
    robot.run(25)
    robot.sending.add("head")
    robot.run(30)
    assert "head" not in robot.recovery.stuck and robot.said()["cameras"]["head"]["state"] == "ok"


def test_each_cameras_usb_position_is_reported_and_one_sharing_a_root_port_across_hubs_is_advised_to_move(tmp_path):
    robot = Robot(tmp_path)
    robot.run(1)
    assert all(camera["usb"] is None and camera["usb_advice"] is None for camera in robot.said()["cameras"].values())
    usb = lambda path: dict(path=path, device=7, speed=5000, hubs=["0bda:0420", "0424:5806"][:path.count(".")])
    # s1u-03: the head behind two hubs on root port 2-3, the torso and the wrists behind another hub on it.
    (tmp_path / "usb").write_text(json.dumps({"head_rgbd": usb("2-3.3.3"), "torso_rgbd": usb("2-3.4.6"),
                                              "left_wrist_rgbd": usb("2-3.4.4"), "right_wrist_rgbd": usb("2-3.4.3"),
                                              "head_stereo": usb("1-4.3.6")}))
    robot.run(1)
    said = robot.said()["cameras"]
    assert said["head"]["usb"] == usb("2-3.3.3")
    assert said["head"]["usb_advice"] == "与 torso、wrist_left、wrist_right 共用 USB 根口 2-3（经 2 级 hub），建议改接独立根口"
    assert said["torso"]["usb_advice"] is None and said["wrist_left"]["usb_advice"] is None  # the head is the odd one
    assert robot.said()["state"] == "ok"  # advice, not a state
    # s1u-02: the head on a root port of its own, the torso and the wrists behind one hub; s1u-01 alike.
    (tmp_path / "usb").write_text(json.dumps({"head_rgbd": usb("2-2"), "torso_rgbd": usb("2-3.2.6"),
                                              "left_wrist_rgbd": usb("2-3.2.4"), "right_wrist_rgbd": usb("2-3.2.3")}))
    robot.run(1)
    assert all(camera["usb_advice"] is None for camera in robot.said()["cameras"].values())
    # Two hubs with a camera each on one root port: both told.
    (tmp_path / "usb").write_text(json.dumps({"head_rgbd": usb("2-3.3.3"), "torso_rgbd": usb("2-3.4.6"),
                                              "left_wrist_rgbd": usb("1-2"), "right_wrist_rgbd": usb("1-3")}))
    robot.run(1)
    said = robot.said()["cameras"]
    assert said["head"]["usb_advice"] == "与 torso 共用 USB 根口 2-3（经 2 级 hub），建议改接独立根口"
    assert said["torso"]["usb_advice"] == "与 head 共用 USB 根口 2-3（经 2 级 hub），建议改接独立根口"
    assert said["wrist_left"]["usb_advice"] is None
    (tmp_path / "observer").mkdir()
    observer = Robot(tmp_path / "observer", "s1u_observer")
    (tmp_path / "observer/usb").write_text(json.dumps({"head_rgbd": usb("2-1.3"), "torso_rgbd": usb("2-3.2.6")}))
    observer.run(1)
    assert [camera["usb"]["path"] for camera in observer.said()["cameras"].values()] == ["2-1.3", "2-3.2.6"]
    assert all(camera["usb_advice"] is None for camera in observer.said()["cameras"].values())


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
    assert robot.said()["state"] == "ok" and not robot.recovery.ladder
    # An orin container that does not answer: after 300 s the reset is asked again after the pause, no driver
    # restart for a reset that did not run, and never stuck for it.
    robot.sending = {"head"}
    robot.run(6 + 30 + 60 + 120 + 240)
    torso = robot.said()["cameras"]["torso"]
    assert torso["step"] == 2 and len(robot.restarts) == 4
    asked = json.loads((tmp_path / "reset-request").read_text())["id"]
    robot.run(300)
    torso = robot.said()["cameras"]["torso"]
    assert torso["last_result"] == "the orin container did not answer" and torso["next_attempt_at"] == round(robot.recovery.wall()) + 480
    assert robot.events[-1] == ("CameraUsbReset", "USB reset of torso: the orin container did not answer", "Warning")
    assert len(robot.restarts) == 4 and not (tmp_path / "reset-request").exists()
    robot.run(479)
    assert not (tmp_path / "reset-request").exists()
    robot.run(1)
    assert json.loads((tmp_path / "reset-request").read_text())["id"] != asked and robot.said()["state"] == "recovering"
    # It did not run on the Orin (ssh away): asked again after 900 s, and the head that works is not blanked.
    request = json.loads((tmp_path / "reset-request").read_text())
    (tmp_path / "reset-result").write_text(json.dumps(dict(id=request["id"], ok=False, detail="ssh: connect timed out")))
    robot.run(1)
    torso = robot.said()["cameras"]["torso"]
    assert torso["last_result"] == "ssh: connect timed out" and torso["step"] == 2 and len(robot.restarts) == 4
    assert torso["next_attempt_at"] == round(robot.recovery.wall()) + 900 and robot.said()["state"] == "recovering"
    # Not on the Orin's USB at all: stuck at once, saying the reset was not done.
    robot.run(900)
    request = json.loads((tmp_path / "reset-request").read_text())
    (tmp_path / "reset-result").write_text(json.dumps(dict(id=request["id"], ok=False, reset=[], missing=["torso_rgbd"],
                                                           detail="no USB device of torso_rgbd")))
    robot.run(1)
    torso = robot.said()["cameras"]["torso"]
    assert robot.said()["state"] == "stuck" and torso["last_result"] == cameras.NO_DEVICE and len(robot.restarts) == 4
    assert json.loads((tmp_path / "diagnose-request").read_text())["cameras"] == ["torso_rgbd"]


def test_the_driver_restart_after_a_usb_reset_counts_for_every_camera_down_with_restarts_to_go(tmp_path):
    """Not one for the reset camera and another for a camera that stalled meanwhile a second later, in the middle of
    the first's driver start."""
    robot = Robot(tmp_path)
    robot.sending = {"torso", "wrist_left", "wrist_right"}
    robot.run(475)
    robot.sending.discard("torso")
    robot.run(35)
    request = json.loads((tmp_path / "reset-request").read_text())
    assert request["cameras"] == ["head_rgbd"] and len(robot.restarts) == 3
    robot.run(5)  # the torso down for 39 s while the orin container works
    (tmp_path / "reset-result").write_text(json.dumps(dict(id=request["id"], ok=True, reset=["head_rgbd"], missing=[],
                                                           detail="reset ok head_rgbd bus 002 dev 012")))
    robot.run(1)
    assert robot.restarts[-1] == robot.now and len(robot.restarts) == 4
    assert robot.events[-1] == ("CameraDriverRestart", "the camera driver restarted for head, torso (after the USB reset of head)", "Normal")
    torso = robot.said()["cameras"]["torso"]
    assert torso["step"] == 1 and torso["attempts"] == 1 and torso["next_attempt_at"] == round(robot.recovery.wall()) + 60
    robot.run(59)
    assert len(robot.restarts) == 4


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
