"""The robot's camera preview (the Addon s1u-robot's video), for the site's camera wall.

While anyone watches (and 5 s after), each camera's newest frame is kept from its ROS topic, with or without a Run;
video_server serves two multipart JPEG streams from it:
- the camera wall's strip: the three views side by side, each preview_width x preview_height, preview_frame_rate a second
  while someone watches it;
- a camera's own frames at full resolution, as the robot's full view: a compressed topic's JPEG passes through untouched.

The robot's CPUs are shared with the SDK and the Station (E13's bar: <= 5% of one core with the wall open, <= 15% with a
full view). The device layer decodes every camera frame anyway, and keeps a 128 x 72 thumbnail of every sixth in
/run/rlark/s1u/preview.bin (SHARED_FRAMES, the contract with the device layer): the strip is made from those, with no topic and no decode. A device that writes
none (and a full view) subscribes the topics: a compressed topic is received raw (rospy.AnyMsg: 1.7% of a core for the
three at 30 fps on s1u_01, 2.9% deserialized), a frame's JPEG taken out only when used, and the strip decodes each view at
1/2-1/8 inside libjpeg (PIL draft: 5.5 ms a strip on s1u_01, 14 ms with the robot's OpenCV 4.2, whose reduced decode is a
full one); s1u_01's wall was 5.5% that way.

Each part names its frame's age (X-Ages: the strip's three cameras' in CAMERAS order, -1 for one never seen), so a viewer
tells a frozen camera from a live one. The synthetic source draws moving test frames, for tests and hosts without ROS.
"""

import os
import struct
import threading
import time
from pathlib import Path

from video import CAMERAS

BOUNDARY = "s1upreview"
# The device layer's thumbnails (a file on the host it writes, this reads only), little-endian: int64 sequence (odd while it writes,
# +2 a write), float64[3] each camera's receive time (unix s), uint8[3, 72, 128, 3] RGB, cameras in CAMERAS order.
SHARED_FRAMES = Path("/host/preview.bin")  # the host's /run/rlark/s1u/preview.bin
SHARED_SHAPE = (3, 72, 128, 3)
SHARED_SIZE = 32 + 3 * 72 * 128 * 3


class Previews:
    def __init__(self, config, clock=time.monotonic):
        import cv2

        self.cv2 = cv2
        self.config = config
        self.clock = clock
        # One lock; a full view's streams wait for a camera's frames, the wall's for the strip.
        self.lock = threading.Lock()
        self.framed, self.stripped = threading.Condition(self.lock), threading.Condition(self.lock)
        # Each camera's newest frame: a compressed topic's serialized message (raw) and its JPEG once taken out, or a raw
        # topic's BGR image and its JPEG once encoded; when it arrived and its sequence number.
        self.frames = {camera: {"raw": None, "jpeg": None, "image": None, "received": None, "sequence": 0} for camera in CAMERAS}
        self.strip = {"jpeg": None, "ages": None, "sequence": 0}
        self.stopping = threading.Event()
        # Open streams, and those of the strip, which is made only while one is open; the topics are subscribed while a
        # stream is open and until IDLE_SECONDS after the last one closed.
        self.streams = self.watchers = 0
        self.idle_since = clock()
        self.subscription, self.subscribers = threading.Lock(), []
        self.shared_frames = Path(config.get("shared_frames", SHARED_FRAMES))

    IDLE_SECONDS = 5

    def start(self):
        if self.config["source"] == "ros":
            import rospy

            rospy.init_node("s1u_preview_" + self.config["robot_identifier"].replace("-", "_"), anonymous=True, disable_signals=True)
        else:
            threading.Thread(target=self.synthesize, daemon=True).start()
        threading.Thread(target=self.compose, daemon=True).start()

    def subscribe(self):
        """The cameras' topics, unless subscribed already (holding the subscription lock)."""
        import rospy
        from cv_bridge import CvBridge
        from sensor_msgs.msg import CompressedImage, Image

        if self.subscribers:
            return
        bridge = CvBridge()
        self.message = CompressedImage
        for camera in self.config["cameras"]:
            topic = self.config["cameras"][camera]
            if topic["compressed"]:
                callback = lambda message, camera=camera: self.receive(camera, raw=message._buff)
            else:
                callback = lambda message, camera=camera: self.receive(camera, image=bridge.imgmsg_to_cv2(message, "bgr8"))
            self.subscribers.append(rospy.Subscriber(topic["topic"], rospy.AnyMsg if topic["compressed"] else Image,
                                                     callback, queue_size=1, buff_size=16 * 1024 * 1024))

    def synthesize(self):
        """Moving test frames at the configured size and rate: a bar sweeping across a camera-coloured field."""
        import numpy

        width, height, rate = self.config["width"], self.config["height"], self.config["frame_rate"]
        colours = {"head": (90, 60, 30), "wrist_left": (30, 90, 60), "wrist_right": (60, 30, 90)}
        tick = 0
        while not self.stopping.wait(1 / rate):
            for camera in self.config["cameras"]:
                image = numpy.full((height, width, 3), colours[camera], numpy.uint8)
                x = tick * 7 % width
                image[:, x:x + width // 16] = 230
                self.cv2.putText(image, "%s %s %d" % (self.config["robot_identifier"], camera, tick), (12, height - 16),
                                 self.cv2.FONT_HERSHEY_SIMPLEX, height / 360, (255, 255, 255), 2)
                self.receive(camera, jpeg=self.cv2.imencode(".jpg", image, [self.cv2.IMWRITE_JPEG_QUALITY, 80])[1].tobytes())
            tick += 1

    def receive(self, camera, raw=None, jpeg=None, image=None):
        with self.lock:
            frame = self.frames[camera]
            frame.update(raw=raw, jpeg=jpeg, image=image, received=self.clock(), sequence=frame["sequence"] + 1)
            self.framed.notify_all()

    def jpeg(self, frame):
        """The frame's JPEG, taken out of its message or encoded from its image the first time it is asked for; None
        before the camera's first frame."""
        if frame["jpeg"] is None and frame["received"] is not None:
            frame["jpeg"] = (bytes(self.message().deserialize(frame["raw"]).data) if frame["raw"] is not None else
                             self.cv2.imencode(".jpg", frame["image"], [self.cv2.IMWRITE_JPEG_QUALITY, 85])[1].tobytes())
        return frame["jpeg"]

    def thumbnail(self, frame, width, height):
        """The frame at the strip's size: libjpeg decodes it at the smallest 1/2-1/8 scale at least that size."""
        import io

        import numpy
        from PIL import Image

        if frame["image"] is not None:
            return self.cv2.resize(frame["image"], (width, height), interpolation=self.cv2.INTER_AREA)
        image = Image.open(io.BytesIO(self.jpeg(frame)))
        image.draft("RGB", (width, height))
        return numpy.asarray(image.convert("RGB").resize((width, height), Image.BILINEAR))[:, :, ::-1]

    def ages(self, now):
        """Each camera's frame age at ``now``; a frame that arrived after ``now`` (while the strip was made) is 0."""
        return [max(0.0, round(now - frame["received"], 2)) if frame["received"] is not None else -1 for frame in self.frames.values()]

    def shared(self):
        """The device layer's thumbnails and their receive times, read whole between two equal, even sequences; None
        while the device layer writes none."""
        import numpy

        try:
            with self.shared_frames.open("rb") as file:
                while True:
                    data = file.read(SHARED_SIZE)
                    file.seek(0)
                    (sequence,) = struct.unpack_from("<q", data)
                    if sequence % 2 == 0 and struct.unpack("<q", os.pread(file.fileno(), 8, 0))[0] == sequence:
                        return struct.unpack_from("<3d", data, 8), numpy.frombuffer(data, numpy.uint8, offset=32).reshape(SHARED_SHAPE)
        except FileNotFoundError:
            return None

    def topics_needed(self):
        """Whether an open stream needs the topics: a full view, or the strip of a device without shared thumbnails."""
        return self.streams > self.watchers or bool(self.watchers) and not self.shared_frames.exists()

    def compose(self):
        """The strip, preview_frame_rate a second while someone watches it: each camera's newest frame, or a dark view
        for a camera that sent nothing within camera_timeout_seconds."""
        import numpy

        width, height = self.config["preview_width"], self.config["preview_height"]
        dark = numpy.full((height, width, 3), 24, numpy.uint8)
        while not self.stopping.wait(1 / self.config["preview_frame_rate"]):
            if self.subscribers and not self.topics_needed() and self.clock() - self.idle_since >= self.IDLE_SECONDS:
                self.unsubscribe()
            if not self.watchers:
                continue
            shared = self.shared()
            if shared:
                # The device's RGB thumbnails, each dark once its camera sent nothing within camera_timeout_seconds.
                now, (received, images) = time.time(), shared
                ages = [round(max(0.0, now - at), 2) if at else -1 for at in received]
                views = [self.cv2.resize(self.cv2.cvtColor(image, self.cv2.COLOR_RGB2BGR), (width, height))
                         if 0 <= age <= self.config["camera_timeout_seconds"] else dark for image, age in zip(images, ages)]
            else:
                now = self.clock()
                with self.lock:
                    frames = [dict(frame) for frame in self.frames.values()]
                ages = self.ages(now)
                views = [self.thumbnail(frame, width, height)
                         if frame["received"] is not None and now - frame["received"] <= self.config["camera_timeout_seconds"]
                         else dark for frame in frames]
            jpeg = self.cv2.imencode(".jpg", numpy.hstack(views), [self.cv2.IMWRITE_JPEG_QUALITY, 70])[1].tobytes()
            with self.lock:
                self.strip.update(jpeg=jpeg, ages=ages, sequence=self.strip["sequence"] + 1)
                self.stripped.notify_all()

    def unsubscribe(self):
        """No stream needed the topics for IDLE_SECONDS: they go, and with them each camera's frame."""
        with self.subscription:
            if self.topics_needed():
                return
            for subscriber in self.subscribers:
                subscriber.unregister()
            self.subscribers = []
            with self.lock:
                for frame in self.frames.values():
                    frame.update(raw=None, jpeg=None, image=None, received=None)

    def next(self, camera, sequence):
        """The newest JPEG after ``sequence`` and its ages: the strip's when ``camera`` is None. A camera that stopped
        sends its last frame again each second, so a viewer sees its age grow."""
        with self.lock:
            source, changed = (self.strip, self.stripped) if camera is None else (self.frames[camera], self.framed)
            changed.wait_for(lambda: source["sequence"] != sequence or self.stopping.is_set(), timeout=1)
            if camera is None:
                return source["jpeg"], source["ages"], source["sequence"]
            jpeg = self.jpeg(source)
            age = [round(self.clock() - source["received"], 2)] if source["received"] is not None else [-1]
            return jpeg, age, source["sequence"]

    def serve(self, handler, camera):
        """Stream to ``handler`` until its client leaves: the strip, or ``camera``'s full frames."""
        with self.lock:
            if self.streams >= self.config["maximum_previews"]:
                raise ValueError("Maximum preview stream count reached")
            self.streams += 1
            self.watchers += camera is None
        try:
            if self.config["source"] == "ros" and self.topics_needed():
                with self.subscription:
                    self.subscribe()
            handler.send_response(200)
            handler.send_header("Content-Type", "multipart/x-mixed-replace; boundary=" + BOUNDARY)
            handler.send_header("Cache-Control", "no-store")
            handler.end_headers()
            sequence = None
            while not self.stopping.is_set():
                jpeg, ages, sequence = self.next(camera, sequence)
                if jpeg is None:
                    continue
                handler.wfile.write(("--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\nX-Ages: %s\r\n\r\n"
                                     % (BOUNDARY, len(jpeg), ",".join(map(str, ages)))).encode() + jpeg + b"\r\n")
        except OSError:
            pass  # The viewer left.
        finally:
            with self.lock:
                self.streams -= 1
                self.watchers -= camera is None
                self.idle_since = self.clock()

    def close(self):
        self.stopping.set()
        with self.lock:
            self.framed.notify_all()
            self.stripped.notify_all()
        if self.config["source"] == "ros":
            import rospy

            for subscriber in self.subscribers:
                subscriber.unregister()
            rospy.signal_shutdown("video service stopping")
