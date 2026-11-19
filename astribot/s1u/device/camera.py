"""Subscribe to an existing ROS camera topic; never start or stop camera drivers."""

import argparse
import json
import os
import select
import signal
import subprocess
import threading
import time
from pathlib import Path

from video import CAMERAS, encoder_arguments, stop_processes


def main():
    import cv2
    import numpy
    import rospy
    from cv_bridge import CvBridge
    from sensor_msgs.msg import CompressedImage, Image

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", required=True)
    parser.add_argument("--camera", choices=CAMERAS, required=True)
    arguments = parser.parse_args()
    config = json.loads(Path(arguments.config).read_text())
    camera = config["cameras"][arguments.camera]
    stopping = threading.Event()
    lock = threading.Lock()
    latest = {"message": None, "received": time.monotonic()}
    bridge = CvBridge()

    def receive(message):
        with lock:
            latest.update(message=message, received=time.monotonic())

    rospy.init_node("s1u_video_" + arguments.camera, anonymous=True, disable_signals=True)
    subscriber = rospy.Subscriber(camera["topic"], CompressedImage if camera["compressed"] else Image,
                                  receive, queue_size=1, buff_size=16 * 1024 * 1024)
    for number in (signal.SIGTERM, signal.SIGINT):
        signal.signal(number, lambda *_: stopping.set())
    destination = "rtsp://127.0.0.1:%s/%s/%s" % (config["publish_port"], config["robot_identifier"], arguments.camera)
    command = [config["ffmpeg"], "-nostdin", "-hide_banner", "-loglevel", "warning",
               "-f", "rawvideo", "-pix_fmt", "bgr24", "-video_size", "%sx%s" % (config["width"], config["height"]),
               "-framerate", str(config["frame_rate"]), "-i", "pipe:0", *encoder_arguments(config), destination]
    process = subprocess.Popen(command, stdin=subprocess.PIPE)
    os.set_blocking(process.stdin.fileno(), False)
    diagnostic = Path(arguments.config).parent / (arguments.camera + ".json")
    last_message = None
    frames = 0
    last_diagnostic = 0.0
    try:
        while not stopping.is_set() and not rospy.is_shutdown():
            with lock:
                message, received = latest["message"], latest["received"]
            if time.monotonic() - received > config["camera_timeout_seconds"]:
                raise RuntimeError("Camera topic stopped producing frames: " + camera["topic"])
            if message is None or message is last_message:
                stopping.wait(0.01)
                continue
            last_message = message
            started = time.monotonic()
            if camera["compressed"]:
                frame = cv2.imdecode(numpy.frombuffer(message.data, dtype=numpy.uint8), cv2.IMREAD_COLOR)
            else:
                frame = bridge.imgmsg_to_cv2(message, desired_encoding="bgr8")
            if frame is None:
                raise RuntimeError("Cannot decode camera frame")
            frame = cv2.resize(frame, (config["width"], config["height"]))
            pending = memoryview(frame.tobytes())
            deadline = time.monotonic() + 2
            while pending and not stopping.is_set():
                if process.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError("Preview encoder is not consuming frames")
                if select.select([], [process.stdin], [], 0.1)[1]:
                    pending = pending[os.write(process.stdin.fileno(), pending):]
            frames += 1
            if time.monotonic() - last_diagnostic >= 1:
                temporary = diagnostic.with_suffix(".temporary")
                temporary.write_text(json.dumps({"frames": frames, "received_at": time.time(),
                                                "source_timestamp": message.header.stamp.to_sec(), "topic": camera["topic"]}))
                temporary.replace(diagnostic)
                last_diagnostic = time.monotonic()
            stopping.wait(max(0, 1 / config["frame_rate"] - (time.monotonic() - started)))
    finally:
        subscriber.unregister()
        process.stdin.close()
        stop_processes(process, timeout=3)
        rospy.signal_shutdown("Preview stopped; camera drivers remain running")


if __name__ == "__main__":
    main()
