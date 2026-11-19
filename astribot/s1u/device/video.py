"""One robot, a publisher per camera of its body, expiring viewing sessions (the Addon s1u-robot's video).

This module neither opens cameras nor issues robot commands. MediaMTX owns
the actual browser connections; only the publishers created here are stopped.
"""

import json
import secrets
import subprocess
import threading
import time
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

# Every camera an S1U body may have, in the preview strip's order; a body's own are config["cameras"].
CAMERAS = ("head", "wrist_left", "wrist_right")
# The robot video's ports on the host's network (astribot/docs/architecture.md, s1u-robot): the authorization callback MediaMTX calls
# (loopback), MediaMTX's WebRTC (the browser's, its address handed out with a viewing session), its ICE (UDP and
# TCP), its RTSP (the camera publishers', loopback) and its API (loopback). Apart from MediaMTX's defaults, which
# other software on a robot may hold.
CONTROL_PORT, MEDIA_PORT, CONNECTION_PORT, PUBLISH_PORT, MEDIA_CONTROL_PORT = 18890, 18889, 18189, 18554, 19997


def stop_processes(*processes, timeout=5):
    """Each process terminated, killed if it outlives ``timeout``."""
    for process in processes:
        if process.poll() is None:
            process.terminate()
    for process in processes:
        try:
            process.wait(timeout)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


def encoder_arguments(config):
    return [
        "-an", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
        "-pix_fmt", "yuv420p", "-profile:v", "baseline", "-bf", "0", "-threads", "1",
        "-g", str(config["frame_rate"]), "-b:v", config["bitrate"],
        "-f", "rtsp", "-rtsp_transport", "tcp",
    ]


class VideoPublishers:
    def __init__(self, config, directory):
        self.config = config
        self.directory = Path(directory)
        self.processes = {}
        self.restarts = {}
        self.errors = {}
        self.started = False

    def start_camera(self, camera):
        config = self.config
        destination = "rtsp://127.0.0.1:%s/%s/%s" % (
            config["publish_port"], config["robot_identifier"], camera,
        )
        if config["source"] == "synthetic":
            command = [
                config["ffmpeg"], "-nostdin", "-hide_banner", "-loglevel", "warning",
                "-re", "-f", "lavfi", "-i",
                "testsrc2=size=%sx%s:rate=%s" % (config["width"], config["height"], config["frame_rate"]),
                *encoder_arguments(config), destination,
            ]
        else:
            command = [
                config["camera_python"], str(Path(__file__).with_name("camera.py")),
                "--config", str(self.directory / "publisher_config.json"), "--camera", camera,
            ]
        with (self.directory / (camera + ".log")).open("ab") as output:
            self.processes[camera] = subprocess.Popen(command, stdout=output, stderr=output)
        self.errors.pop(camera, None)

    def start(self):
        if self.started:
            return
        self.started = True
        self.restarts = {camera: 0 for camera in self.config["cameras"]}
        try:
            for camera in self.config["cameras"]:
                self.start_camera(camera)
        except Exception:
            self.stop()
            raise

    def maintain(self):
        if not self.started:
            return
        for camera, process in list(self.processes.items()):
            if process.poll() is None:
                continue
            self.errors[camera] = "Publisher exited with code %s; see %s.log" % (process.returncode, camera)
            if self.restarts[camera] < self.config["maximum_restarts"]:
                self.restarts[camera] += 1
                self.start_camera(camera)

    def stop(self):
        self.started = False
        stop_processes(*self.processes.values())
        self.processes.clear()

    def snapshot(self):
        result = {}
        for camera in self.config["cameras"]:
            process = self.processes.get(camera)
            status = {"publisher_alive": process is not None and process.poll() is None,
                      "restarts": self.restarts.get(camera, 0), "error": self.errors.get(camera)}
            path = self.directory / (camera + ".json")
            if path.exists():
                # camera.py replaces the diagnostic atomically.
                status["source"] = json.loads(path.read_text())
            result[camera] = status
        return result


class ViewingSessions:
    def __init__(self, config, publishers, clock=time.monotonic, kick=None):
        self.config = config
        self.publishers = publishers
        self.clock = clock
        self.kick = kick or self.kick_media_session
        self.sessions = {}
        self.lock = threading.RLock()
        self.idle_since = None
        self.pending_kicks = set()

    def create(self):
        with self.lock:
            self.expire()
            if len(self.sessions) >= self.config["maximum_viewers"]:
                raise ValueError("Maximum viewer count reached")
            self.publishers.start()
            identifier = secrets.token_urlsafe(24)
            token = secrets.token_urlsafe(32)
            self.sessions[identifier] = {"token": token, "expires": self.clock() + self.config["viewer_timeout_seconds"], "media_sessions": set()}
            self.idle_since = None
            base = self.config["public_media_address"].rstrip("/")
            return {"viewer_identifier": identifier, "viewer_token": token,
                    "source": self.config["source"], "heartbeat_seconds": 10,
                    "streams": [{"camera": camera, "address": "%s/%s/%s/whep" % (base, self.config["robot_identifier"], camera)} for camera in self.config["cameras"]]}

    def refresh(self, identifier):
        with self.lock:
            self.expire()
            session = self.sessions[identifier]
            session["expires"] = self.clock() + self.config["viewer_timeout_seconds"]
            return self.snapshot()

    def remove(self, identifier):
        with self.lock:
            session = self.sessions.pop(identifier, None)
            if session:
                self.pending_kicks.update(session["media_sessions"])
                self.retry_kicks()
            if not self.sessions and self.idle_since is None:
                self.idle_since = self.clock()

    def expire(self):
        for identifier, session in list(self.sessions.items()):
            if session["expires"] <= self.clock():
                self.remove(identifier)

    def authorize(self, request):
        with self.lock:
            self.expire()
            paths = [self.config["robot_identifier"] + "/" + camera for camera in self.config["cameras"]]
            if request.get("path") not in paths:
                return False
            if request.get("action") == "publish":
                return request.get("ip") in ("127.0.0.1", "::1") and request.get("protocol") == "rtsp"
            if request.get("action") != "read" or request.get("protocol") != "webrtc":
                return False
            token = request.get("token", "")
            if not isinstance(token, str) or not token:
                return False
            for session in self.sessions.values():
                if secrets.compare_digest(token, session["token"]):
                    if request.get("id"):
                        session["media_sessions"].add(request["id"])
                    return True
            return False

    def kick_media_session(self, identifier):
        request = Request("http://127.0.0.1:%s/v3/webrtcsessions/kick/%s" % (self.config["media_control_port"], identifier), data=b"", method="POST")
        try:
            with urlopen(request, timeout=0.5):
                return True
        except HTTPError as error:
            return error.code == 404  # Already closed by the browser or MediaMTX.
        except OSError:
            return False

    def retry_kicks(self):
        for identifier in list(self.pending_kicks):
            if self.kick(identifier):
                self.pending_kicks.discard(identifier)

    def tick(self):
        with self.lock:
            self.expire()
            self.retry_kicks()
            if not self.sessions and self.idle_since is not None and self.clock() - self.idle_since >= self.config["idle_grace_seconds"]:
                self.publishers.stop()
                self.idle_since = None
            if self.sessions:
                self.publishers.maintain()

    def snapshot(self):
        with self.lock:
            return {"robot_identifier": self.config["robot_identifier"], "viewer_count": len(self.sessions),
                    "source": self.config["source"], "cameras": self.publishers.snapshot()}

    def close(self):
        with self.lock:
            for identifier in list(self.sessions):
                self.remove(identifier)
            self.publishers.stop()
