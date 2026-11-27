"""Read-only S1U ROS1 observer. Serves one bounded snapshot on the Pod network."""

import json
import math
import os
import struct
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import urlsplit

JOINT_TYPE = ("astribot_msgs/RobotJointState", "45165dcf13478cde61ebfe1f96d06307")
ENDPOINT_TYPE = ("astribot_msgs/RobotCartesianState", "44bea01d596ff699fa1447bec34167ac")
STALE_SECONDS = 2.0


def decode(data, dimensions):
    """Decode the two pinned ROS wire types, retaining only the advertised fields."""
    if len(data) > 4096:
        raise ValueError("message exceeds limit")
    _, seconds, nanos, frame_size = struct.unpack_from("<4I", data)
    if nanos >= 1_000_000_000 or frame_size > 128 or 16 + frame_size > len(data):
        raise ValueError("invalid ROS header")
    frame = data[16:16 + frame_size].decode("utf-8")
    offset = 16 + frame_size
    sample = {"sourceUnixSeconds": seconds + nanos / 1e9} if seconds or nanos else {}
    if dimensions is None:
        pose = struct.unpack_from("<7d", data, offset)
        if not all(map(math.isfinite, pose)) or not math.isclose(math.hypot(*pose[3:]), 1.0, abs_tol=0.01):
            raise ValueError("invalid pose")
        sample.update(position=pose[:3], quaternion=pose[3:])
        if frame:
            sample["frame"] = frame
        return sample

    if offset == len(data):
        raise ValueError("missing joint mode")
    offset += 1  # mode
    names, = struct.unpack_from("<I", data, offset)
    offset += 4
    if names not in (0, dimensions):
        raise ValueError("invalid joint name count")
    for _ in range(names):
        size, = struct.unpack_from("<I", data, offset)
        offset += 4 + size
    for field in ("position", "velocity"):
        count, = struct.unpack_from("<I", data, offset)
        offset += 4
        if count not in (0, dimensions):
            raise ValueError("wrong joint vector size")
        values = struct.unpack_from("<%dd" % count, data, offset)
        offset += count * 8
        if values and all(map(math.isfinite, values)):
            sample[field] = values
    if "position" not in sample and "velocity" not in sample:
        raise ValueError("no valid joint vectors")
    return sample


class Latest:
    def __init__(self):
        self.samples = {}
        self.lock = threading.Lock()

    def record(self, key, sample, now):
        source = sample.get("sourceUnixSeconds")
        with self.lock:
            previous = self.samples.get(key)
            if previous and source is not None:
                last = previous[0].get("sourceUnixSeconds")
                if last is not None and (source == last or (source < last and now - previous[1] <= STALE_SECONDS)):
                    return
            self.samples[key] = sample, now

    def read(self, key, now):
        with self.lock:
            previous = self.samples.get(key)
        if previous is None:
            return {"id": key[1], "state": "waiting"}
        sample, arrived = previous
        age = max(0.0, now - arrived)
        if age > STALE_SECONDS:
            return {"id": key[1], "state": "stale", "ageSeconds": age}
        return {"id": key[1], "state": "live", "ageSeconds": age, **sample}


def subscribe(rospy, profile, body, uid):
    """Attach read-only ROS subscriptions and return a snapshot reader."""
    if profile["version"] != 1 or body not in {variant["id"] for variant in profile["variants"]}:
        raise ValueError("unsupported body profile")
    groups = {group["id"]: group["dimensions"] for group in profile["jointGroups"]}
    endpoints = profile["endpoints"]
    latest = Latest()

    def receive(raw, subscription):
        key, wire_type, size = subscription
        headers = raw._connection_header
        if (headers.get("type"), headers.get("md5sum")) != wire_type:
            rospy.logwarn_throttle(60, "S1U body observer received an unsupported ROS message type")
            return
        try:
            sample = decode(raw._buff, size)
        except (ValueError, struct.error):
            rospy.logwarn_throttle(60, "S1U body observer received an invalid ROS state sample")
            return
        latest.record(key, sample, time.monotonic())

    subscriptions = [
        rospy.Subscriber("/astribot_%s/joint_space_states" % key, rospy.AnyMsg, receive,
                         (("joint", key), JOINT_TYPE, size), queue_size=1)
        for key, size in groups.items()
    ] + [
        rospy.Subscriber("/astribot_%s/endpoint_current_states" % key, rospy.AnyMsg, receive,
                         (("endpoint", key), ENDPOINT_TYPE, None), queue_size=1)
        for key in endpoints
    ]
    def snapshot():
        now = time.monotonic()
        return {
            "version": profile["version"], "nodeUID": uid, "body": body,
            "joints": [latest.read(("joint", group), now) for group in groups],
            "endpoints": [latest.read(("endpoint", point), now) for point in endpoints],
        }

    return snapshot, subscriptions


def main():
    import rospy

    profile = json.loads(Path(os.environ["S1U_CAPABILITIES"]).read_text())
    body = os.environ["S1U_BODY"]
    uid = os.environ["S1U_NODE_UID"]
    host = os.environ["S1U_OBSERVER_ADDRESS"]
    rospy.init_node("rlark_body_observer", anonymous=True, disable_rosout=True, disable_rostime=True)
    master = rospy.get_master()
    _, _, initial_master_pid = master.getPid()

    def watch_master():
        while not rospy.is_shutdown():
            time.sleep(2)
            try:
                code, _, pid = master.getPid()
            except OSError:
                continue
            if code == 1 and pid != initial_master_pid:
                rospy.signal_shutdown("ROS master restarted")
                return

    threading.Thread(target=watch_master, daemon=True).start()
    snapshot, subscriptions = subscribe(rospy, profile, body, uid)

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, _format, *_args):
            pass

        def do_GET(self):
            if urlsplit(self.path).path.rstrip("/") != "/body-state":
                self.send_error(404)
                return
            payload = json.dumps(snapshot(), allow_nan=False).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

    with HTTPServer((host, 18892), Handler) as server:
        # Return control regularly so SIGTERM and rospy shutdown can unregister all topics.
        server.timeout = 0.5
        while not rospy.is_shutdown():
            server.handle_request()
    for subscription in subscriptions:
        subscription.unregister()


if __name__ == "__main__":
    main()
