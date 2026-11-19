"""The robot's camera video (astribot/s1u/device: video.py, video_server.py, preview.py): viewing sessions, MediaMTX's
authorization, a body's cameras, the camera wall's strip from the device layer's shared thumbnails."""
import http.client
import json
import socket
import struct
import sys
import threading
import time
from pathlib import Path
from types import SimpleNamespace

import cv2
import numpy
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "device"))
import video_server  # noqa: E402
from preview import SHARED_SHAPE, Previews  # noqa: E402
from video import ViewingSessions  # noqa: E402


@pytest.fixture
def viewing():
    clock = [100.0]
    publishers = SimpleNamespace(started=False, starts=0, stops=0)

    def start():
        if not publishers.started:
            publishers.starts += 1
            publishers.started = True

    def stop():
        if publishers.started:
            publishers.stops += 1
            publishers.started = False

    publishers.start, publishers.stop = start, stop
    publishers.maintain = lambda: None
    publishers.snapshot = lambda: {}
    kicked = []
    config = dict(robot_identifier="s1u-01-x86", source="synthetic", public_media_address="http://10.254.240.3:8889",
                  viewer_timeout_seconds=45, idle_grace_seconds=5, maximum_viewers=2,
                  cameras={"head": {}, "wrist_left": {}, "wrist_right": {}})
    sessions = ViewingSessions(config, publishers, clock=lambda: clock[0], kick=lambda identifier: kicked.append(identifier) or True)
    return sessions, publishers, clock, kicked


def test_viewers_share_publishers_and_last_departure_stops_after_grace(viewing):
    sessions, publishers, clock, kicked = viewing
    first, second = sessions.create(), sessions.create()
    assert publishers.starts == 1
    assert [stream["address"] for stream in first["streams"]][0] == "http://10.254.240.3:8889/s1u-01-x86/head/whep"
    with pytest.raises(ValueError, match="Maximum"):
        sessions.create()
    sessions.remove(first["viewer_identifier"])
    clock[0] += 6
    sessions.tick()
    assert publishers.stops == 0
    sessions.remove(second["viewer_identifier"])
    sessions.tick()
    clock[0] += 5
    sessions.tick()
    assert publishers.stops == 1


def test_authorization_expiration_and_failed_disconnect_retry(viewing):
    sessions, publishers, clock, kicked = viewing
    viewer = sessions.create()
    request = dict(action="read", protocol="webrtc", path="s1u-01-x86/head", token=viewer["viewer_token"], id="connection_one")
    assert sessions.authorize(request)
    assert not sessions.authorize({**request, "path": "other_robot/head"})
    assert not sessions.authorize({**request, "token": "wrong"})
    assert not sessions.authorize({**request, "protocol": "rtsp"})
    assert not sessions.authorize(dict(action="publish", protocol="rtsp", path="s1u-01-x86/head", ip="192.168.0.3"))
    assert sessions.authorize(dict(action="publish", protocol="rtsp", path="s1u-01-x86/head", ip="127.0.0.1"))
    sessions.kick = lambda identifier: False
    clock[0] += 46
    sessions.tick()
    assert not sessions.authorize(request)
    assert sessions.pending_kicks == {"connection_one"}
    with pytest.raises(KeyError):
        sessions.refresh(viewer["viewer_identifier"])
    sessions.kick = lambda identifier: kicked.append(identifier) or True
    sessions.tick()
    assert kicked == ["connection_one"] and not sessions.pending_kicks


def test_a_bodys_cameras_are_the_only_ones_published_and_served(viewing):
    observer = video_server.robot_config("s1u-02-x86", "10.254.240.7", ["head"])
    assert list(observer["cameras"]) == ["head"]
    assert observer["cameras"]["head"]["topic"] == "/astribot_camera/head_rgbd/color_compress/compressed"
    media = video_server.media_config(observer)
    assert list(media["paths"]) == ["s1u-02-x86/head"] and media["webrtcAdditionalHosts"] == ["10.254.240.7"]
    assert media["authHTTPAddress"] == "http://127.0.0.1:18890/media/authorize" and media["apiAddress"].startswith("127.0.0.1:")
    full = video_server.robot_config("s1u-01-x86", "10.254.240.3", ["head", "wrist_left", "wrist_right"])
    assert full["cameras"]["wrist_left"]["topic"] == "/astribot_camera/left_wrist_rgbd/color_compress/compressed"
    with pytest.raises(ValueError, match="cameras"):
        video_server.robot_config("s1u-01-x86", "10.254.240.3", [])
    sessions = ViewingSessions(observer, viewing[1])
    assert [stream["camera"] for stream in sessions.create()["streams"]] == ["head"]


def unix_call(path, method, url):
    class Connection(http.client.HTTPConnection):
        def connect(self):
            self.sock = socket.socket(socket.AF_UNIX)
            self.sock.connect(str(path))

    connection = Connection("localhost")
    connection.request(method, url)
    return connection.getresponse()


def test_the_control_is_the_unix_socket_and_the_loopback_port_answers_mediamtx_only(tmp_path, viewing):
    sessions = viewing[0]
    control, authorization = video_server.handlers(sessions, None)
    unix = video_server.UnixServer(str(tmp_path / "s1u-video.sock"), control)
    loopback = video_server.ThreadingHTTPServer(("127.0.0.1", 0), authorization)
    for server in (unix, loopback):
        threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        response = unix_call(tmp_path / "s1u-video.sock", "POST", "/viewers")
        viewer = json.loads(response.read())
        assert response.status == 201 and viewer["viewer_token"]
        connection = http.client.HTTPConnection("127.0.0.1", loopback.server_port)
        connection.request("POST", "/viewers")
        assert connection.getresponse().status == 404  # no viewer is made from the loopback port
        connection = http.client.HTTPConnection("127.0.0.1", loopback.server_port)
        body = json.dumps(dict(action="read", protocol="webrtc", path="s1u-01-x86/head", token=viewer["viewer_token"]))
        connection.request("POST", "/media/authorize", body=body, headers={"Content-Type": "application/json"})
        assert connection.getresponse().status == 200
    finally:
        for server in (unix, loopback):
            server.shutdown()


def test_the_strip_reads_the_device_layers_shared_thumbnails(tmp_path):
    """With the device layer's thumbnails (/run/rlark/s1u/preview.bin), the strip is made from them: each camera's RGB
    view in CAMERAS order, dark once its receive time is older than camera_timeout_seconds."""
    images = numpy.zeros(SHARED_SHAPE, numpy.uint8)
    images[0, ..., 0], images[1, ..., 1], images[2, ..., 2] = 200, 200, 200
    shared = tmp_path / "preview.bin"
    shared.write_bytes(struct.pack("<q3d", 4, time.time(), time.time(), time.time() - 60) + images.tobytes())
    config = dict(robot_identifier="s1u-01-x86", source="synthetic", width=320, height=180, frame_rate=10, preview_width=128,
                  preview_height=72, preview_frame_rate=5, maximum_previews=8, camera_timeout_seconds=10,
                  shared_frames=str(shared), cameras={"head": {}, "wrist_left": {}, "wrist_right": {}})
    previews = Previews(config)
    previews.start()
    control, _ = video_server.handlers(None, previews)
    server = video_server.UnixServer(str(tmp_path / "s1u-video.sock"), control)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        response = unix_call(tmp_path / "s1u-video.sock", "GET", "/preview")
        response.readline()
        headers = dict(line.decode().strip().split(": ", 1) for line in iter(response.readline, b"\r\n"))
        strip = cv2.imdecode(numpy.frombuffer(response.read(int(headers["Content-Length"])), numpy.uint8), cv2.IMREAD_COLOR)
        ages = [float(age) for age in headers["X-Ages"].split(",")]
        assert ages[0] < 2 and ages[1] < 2 and ages[2] > 50
        means = [strip[:, 128 * index:128 * (index + 1)].reshape(-1, 3).mean(0) for index in range(3)]
        assert means[0][2] > 150 and means[0][1] < 50 and means[1][1] > 150 and means[1][2] < 50 and means[2].max() < 50
    finally:
        previews.close()
        server.shutdown()
