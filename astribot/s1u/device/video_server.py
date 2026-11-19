"""The robot's camera video, a container of the Addon s1u-robot on every S1U robot x86. Python 3.8, FFmpeg, MediaMTX.

Two previews: a browser's WebRTC viewing session (ViewingSessions, MediaMTX) and multipart JPEG streams for the site's
camera wall (preview.Previews: the three-view strip, and a camera's full frames).

Its control is a Unix socket only (S1U_SERVICE_SOCKET); RLark reaches it on the node and decides who may watch:
    GET    /status            the publishers and the viewer count
    POST   /viewers           a viewing session: {viewer_identifier, viewer_token, heartbeat_seconds, streams:[{camera,
                              address: the WHEP URL of the robot's MediaMTX}]}
    PUT    /viewers/<id>      renewed; DELETE /viewers/<id> closed
    GET    /preview           the camera wall's strip; GET /preview/<camera> a camera's full frames
The browser then talks WebRTC to the robot's MediaMTX (:18889, video.MEDIA_PORT) with the session's token, which MediaMTX checks with this
service on a loopback-only port; no origin is trusted for itself (a stream is the token's, short-lived).

The cameras are the robot's body's (its Node's label rlark.io/robot-body, the Addon's /etc/rlark/s1u/bodies.json), each
from the vendor's compressed ROS topic. Its processes run on the device layer's CPUs off the SDK core (the profile).
"""

import json
import os
import re
import signal
import socketserver
import ssl
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse
from urllib.request import Request, urlopen

from preview import Previews
from video import (CAMERAS, CONNECTION_PORT, CONTROL_PORT, MEDIA_CONTROL_PORT, MEDIA_PORT, PUBLISH_PORT,
                   VideoPublishers, ViewingSessions, stop_processes)

# Each camera of the S1U by its vendor driver's name, as the SDK knows it, and the view it is.
TOPICS = {"head": "head_rgbd", "wrist_left": "left_wrist_rgbd", "wrist_right": "right_wrist_rgbd"}
ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"


def cpus(text):
    return {cpu for part in text.split(",") for cpu in range(int(part.split("-")[0]), int(part.split("-")[-1]) + 1)}


def body_of(node):
    """The robot's body, its Node's label rlark.io/robot-body (the admission record's)."""
    request = Request("https://%s:%s/api/v1/nodes/%s" % (os.environ["KUBERNETES_SERVICE_HOST"],
                                                        os.environ["KUBERNETES_SERVICE_PORT"], node),
                      headers={"Authorization": "Bearer " + open(ACCOUNT + "/token").read()})
    with urlopen(request, context=ssl.create_default_context(cafile=ACCOUNT + "/ca.crt"), timeout=10) as reply:
        return json.load(reply)["metadata"].get("labels", {})["rlark.io/robot-body"]


def robot_config(node, address, views):
    """The service on a robot: its node, the address a viewer's media reaches it at, its body's cameras."""
    return read_config(dict(
        robot_identifier=node, source="ros", public_media_address="http://%s:%d" % (address, MEDIA_PORT),
        cameras={view: {"topic": "/astribot_camera/%s/color_compress/compressed" % TOPICS[view], "compressed": True}
                 for view in views}))


def read_config(config):
    defaults = dict(control_port=CONTROL_PORT, publish_port=PUBLISH_PORT,
                    media_control_port=MEDIA_CONTROL_PORT, media_port=MEDIA_PORT, connection_port=CONNECTION_PORT,
                    width=640, height=360, frame_rate=15, bitrate="1500k",
                    viewer_timeout_seconds=45, idle_grace_seconds=5, maximum_viewers=8,
                    maximum_restarts=3, camera_timeout_seconds=10,
                    # The camera wall's strip: three views of preview_width x preview_height, preview_frame_rate a second.
                    preview_width=128, preview_height=72, preview_frame_rate=5, maximum_previews=8,
                    ffmpeg="ffmpeg", mediamtx="mediamtx", camera_python=sys.executable)
    config = {**defaults, **config}
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,80}", config["robot_identifier"]):
        raise ValueError("Invalid robot_identifier")
    if config["source"] not in ("ros", "synthetic"):
        raise ValueError("source must be ros or synthetic")
    if not config["cameras"] or set(config["cameras"]) - set(CAMERAS):
        raise ValueError("cameras must be some of %s" % ", ".join(CAMERAS))
    for name in ("control_port", "publish_port", "media_control_port", "media_port", "connection_port"):
        if not isinstance(config[name], int) or not 1024 <= config[name] <= 65535:
            raise ValueError("Invalid " + name)
    for name in ("width", "height", "frame_rate", "maximum_viewers", "camera_timeout_seconds", "preview_width",
                 "preview_height", "preview_frame_rate", "maximum_previews"):
        if not isinstance(config[name], int) or config[name] <= 0:
            raise ValueError("Invalid " + name)
    if config["width"] % 2 or config["height"] % 2 or not 30 <= config["viewer_timeout_seconds"] <= 300:
        raise ValueError("Use even image dimensions and viewer_timeout_seconds between 30 and 300")
    if not 0 <= config["idle_grace_seconds"] <= 60:
        raise ValueError("Invalid idle_grace_seconds")
    address = urlparse(config["public_media_address"])
    if address.scheme not in ("http", "https") or not address.hostname or address.username:
        raise ValueError("Invalid public_media_address")
    if config["source"] == "ros":
        for camera, topic in config["cameras"].items():
            if not topic["topic"].startswith("/"):
                raise ValueError("Configure an absolute ROS topic for " + camera)
            topic.setdefault("compressed", False)
    return config


def media_config(config):
    return {
        "logLevel": "warn", "authMethod": "http",
        "authHTTPAddress": "http://127.0.0.1:%s/media/authorize" % config["control_port"],
        "authHTTPExclude": [{"action": "api"}],
        "api": True, "apiAddress": "127.0.0.1:%s" % config["media_control_port"],
        "rtspAddress": "127.0.0.1:%s" % config["publish_port"], "rtspTransports": ["tcp"],
        "rtmp": False, "hls": False, "srt": False, "moq": False,
        "webrtcAddress": ":%s" % config["media_port"],
        "webrtcLocalUDPAddress": ":%s" % config["connection_port"],
        "webrtcLocalTCPAddress": ":%s" % config["connection_port"],
        "webrtcAdditionalHosts": [urlparse(config["public_media_address"]).hostname],
        # A stream is read with a session's token only; the page that holds it may be any.
        "webrtcAllowOrigins": ["*"],
        "paths": {config["robot_identifier"] + "/" + camera: {} for camera in config["cameras"]},
    }


class Handler(BaseHTTPRequestHandler):
    """Both servers' requests; ``control`` is the Unix socket's (RLark's), else the loopback authorization's."""
    sessions = previews = None
    control = False

    def setup(self):
        super().setup()
        self.connection.settimeout(5)

    def log_message(self, format, *arguments):
        pass  # Request URLs and tokens do not belong in the process log.

    def address_string(self):
        return "unix" if self.control else super().address_string()

    def respond(self, status, body):
        encoded = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def dispatch(self):
        try:
            if not self.control:
                if self.path != "/media/authorize" or self.command != "POST":
                    return self.respond(404, {"detail": "Not found"})
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= 16384:
                    return self.respond(400, {"detail": "Invalid body size"})
                return self.respond(200 if self.sessions.authorize(json.loads(self.rfile.read(length))) else 403, {})
            if self.path == "/status" and self.command == "GET":
                return self.respond(200, self.sessions.snapshot())
            match = re.fullmatch(r"/preview(?:/(%s))?" % "|".join(CAMERAS), self.path)
            if match and self.command == "GET":
                return self.previews.serve(self, match[1])
            if self.path == "/viewers" and self.command == "POST":
                return self.respond(201, self.sessions.create())
            match = re.fullmatch(r"/viewers/([A-Za-z0-9_-]+)", self.path)
            if match and self.command == "PUT":
                return self.respond(200, self.sessions.refresh(match[1]))
            if match and self.command == "DELETE":
                self.sessions.remove(match[1])
                return self.respond(200, {"removed": True})
            return self.respond(404, {"detail": "Not found"})
        except KeyError:
            self.respond(404, {"detail": "Viewer expired"})
        except ValueError as error:
            self.respond(400, {"detail": str(error)})
        except OSError as error:
            self.respond(503, {"detail": str(error)})

    do_GET = do_POST = do_PUT = do_DELETE = dispatch


def handlers(sessions, previews):
    """(the Unix socket's handler, the loopback authorization's)."""
    control = type("Control", (Handler,), dict(sessions=sessions, previews=previews, control=True))
    authorization = type("Authorization", (Handler,), dict(sessions=sessions, previews=previews, control=False))
    return control, authorization


class UnixServer(socketserver.ThreadingMixIn, socketserver.UnixStreamServer):
    daemon_threads = True


def main():
    lines = (line.split("=", 1) for line in open(os.environ["S1U_NODE_PROFILE"]).read().splitlines()
             if "=" in line and line[0] != "#")
    profile = {key: value.strip('"') for key, value in lines}
    # It, ffmpeg and MediaMTX on the device layer's CPUs, off the SDK command thread's core.
    os.sched_setaffinity(0, cpus(profile["S1U_DEVICE_CPUS"]) - cpus(profile["S1U_SDK_CPUS"]))
    os.environ.update(ROS_MASTER_URI="http://%s:11311" % profile["S1U_ROS_IP_ADDRESS"], ROS_IP=profile["S1U_ROS_IP_ADDRESS"])
    node = os.environ["NODE_NAME"]
    views = json.loads(Path(os.environ["S1U_BODIES"]).read_text())[body_of(node)]
    config = robot_config(node, os.environ["S1U_NODE_ADDRESS"], views)
    directory = Path(os.environ["S1U_VIDEO_DIRECTORY"])
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "publisher_config.json").write_text(json.dumps(config))
    configuration = directory / "mediamtx.json"
    configuration.write_text(json.dumps(media_config(config)))
    sessions = ViewingSessions(config, VideoPublishers(config, directory))
    previews = Previews(config)  # the camera wall's runs from start to stop, whether anyone watches or not
    control, authorization = handlers(sessions, previews)
    socket = Path(os.environ["S1U_SERVICE_SOCKET"])
    socket.parent.mkdir(parents=True, exist_ok=True)
    socket.unlink(missing_ok=True)
    servers = [UnixServer(str(socket), control), ThreadingHTTPServer(("127.0.0.1", config["control_port"]), authorization)]
    os.chmod(socket, 0o600)
    stopping = threading.Event()
    for number in (signal.SIGTERM, signal.SIGINT):
        signal.signal(number, lambda *_: stopping.set())
    for server in servers:
        server.daemon_threads = True
        threading.Thread(target=server.serve_forever, daemon=True).start()
    previews.start()  # after the socket: with the robot's master down it waits for it, and the status says so
    process = None
    try:
        with (directory / "mediamtx.log").open("ab") as output:
            process = subprocess.Popen([config["mediamtx"], str(configuration)], stdout=output, stderr=output)
        for attempt in range(30):
            if process.poll() is not None:
                raise RuntimeError("MediaMTX exited; see mediamtx.log")
            try:
                with urlopen("http://127.0.0.1:%s/v3/paths/list" % config["media_control_port"], timeout=1):
                    break
            except OSError:
                if stopping.wait(0.1):
                    return
        else:
            raise RuntimeError("MediaMTX control service did not become ready")
        print("Video of %s (%s) on %s" % (node, ", ".join(config["cameras"]), socket), flush=True)
        while not stopping.wait(0.5):
            if process.poll() is not None:
                raise RuntimeError("MediaMTX exited; see mediamtx.log")
            sessions.tick()
    finally:
        previews.close()
        for server in servers:
            server.shutdown()
            server.server_close()
        sessions.close()
        if process is not None:
            stop_processes(process)


if __name__ == "__main__":
    main()
