"""Client for ``camera.controller.v1.CameraController``.

Wraps the generated gRPC stub with a Pythonic, Unix-socket-aware API that
mirrors the ``camctr`` CLI. Camera lifecycle (open / close), single-frame
capture, and the real-time ``WatchFrames`` stream are exposed as plain methods.
"""

from __future__ import annotations

from collections.abc import Iterator

import grpc

from . import _transport
from ._transport import DEFAULT_CAMERA_SOCKET, DEFAULT_TIMEOUT, LONG_TIMEOUT
from .gen.cameracontroller.v1 import camera_pb2 as pb
from .gen.cameracontroller.v1 import camera_pb2_grpc as pb_grpc

# Re-export the proto enum for convenient ``from embodied_runtime import CameraState``.
CameraState = pb.CameraState

# Human-readable names mirroring cmd/camctr/client.go stateStr().
_STATE_NAMES = {
    CameraState.CAMERA_STATE_UNSPECIFIED: "unknown",
    CameraState.CAMERA_STATE_CLOSED: "closed",
    CameraState.CAMERA_STATE_OPEN: "open",
    CameraState.CAMERA_STATE_ERROR: "error",
}


def state_name(state: int) -> str:
    """Return a lowercase human-readable name for a ``CameraState`` value."""
    return _STATE_NAMES.get(state, "unknown")


class CameraClient:
    """gRPC client for the camera-controller ``CameraController`` service.

    By default it connects to the node-local Unix socket
    ``/var/run/rlark/camera-ctrl.sock`` (matching ``camctr``). Pass ``address``
    to reach a remote TCP server instead.

    Example::

        from embodied_runtime import CameraClient

        with CameraClient() as cam:
            cam.open_camera("camera-0", encoding="h264")
            for frame in cam.watch_frames("camera-0"):
                print(frame.sequence, frame.encoding, len(frame.data))
    """

    def __init__(
        self,
        socket_path: str | None = None,
        *,
        address: str | None = None,
        timeout: float = DEFAULT_TIMEOUT,
    ) -> None:
        self._target = _transport.resolve_target(socket_path, DEFAULT_CAMERA_SOCKET, address)
        self._default_timeout = timeout
        self._channel: grpc.Channel | None = None
        self._stub: pb_grpc.CameraControllerStub | None = None

    # -- connection management ------------------------------------------------

    def connect(self) -> CameraClient:
        """Open the gRPC channel and create the stub. Idempotent."""
        if self._channel is None:
            self._channel = _transport.create_channel(self._target)
            self._stub = pb_grpc.CameraControllerStub(self._channel)
        return self

    def close(self) -> None:
        """Close the underlying gRPC channel."""
        if self._channel is not None:
            self._channel.close()
            self._channel = None
            self._stub = None

    def __enter__(self) -> CameraClient:
        return self.connect()

    def __exit__(self, exc_type, exc, tb) -> None:
        self.close()

    @property
    def stub(self) -> pb_grpc.CameraControllerStub:
        """The raw gRPC stub for advanced / streaming use."""
        if self._stub is None:
            raise RuntimeError("client is not connected; call connect() or use 'with'")
        return self._stub

    # -- camera lifecycle ----------------------------------------------------

    def list_cameras(self, *, timeout: float | None = None) -> pb.ListCamerasResponse:
        """Return all managed cameras with their current state."""
        return self.stub.ListCameras(pb.ListCamerasRequest(), timeout=self._t(timeout))

    def get_camera_info(
        self, camera_id: str, *, timeout: float | None = None
    ) -> pb.GetCameraInfoResponse:
        """Return detailed information about a specific camera."""
        return self.stub.GetCameraInfo(
            pb.GetCameraInfoRequest(camera_id=camera_id), timeout=self._t(timeout)
        )

    def open_camera(
        self,
        camera_id: str,
        *,
        width: int | None = None,
        height: int | None = None,
        fps: int | None = None,
        encoding: str | None = None,
        timeout: float | None = None,
    ) -> pb.OpenCameraResponse:
        """Start frame capture on a camera.

        ``width`` / ``height`` / ``fps`` / ``encoding`` are optional per-open
        overrides; when omitted the camera's defaults are used. ``encoding`` is
        one of ``"jpeg"`` (default), ``"png"``, ``"bmp"``, ``"tiff"`` (frame
        mode — one complete, independently decodable still-image frame per
        message), or ``"h264"``, ``"h265"`` (bitstream mode — Annex B
        elementary-stream chunks).
        """
        req = pb.OpenCameraRequest(camera_id=camera_id)
        if width is not None:
            req.width = width
        if height is not None:
            req.height = height
        if fps is not None:
            req.fps = fps
        if encoding is not None:
            req.encoding = encoding
        return self.stub.OpenCamera(req, timeout=self._t(timeout))

    def close_camera(
        self, camera_id: str, *, timeout: float | None = None
    ) -> pb.CloseCameraResponse:
        """Stop frame capture on a camera."""
        return self.stub.CloseCamera(
            pb.CloseCameraRequest(camera_id=camera_id), timeout=self._t(timeout)
        )

    def capture_frame(
        self,
        camera_id: str,
        *,
        timeout: float | None = None,
        wait: float | None = None,
    ) -> pb.CaptureFrameResponse:
        """Capture and return the most recent frame from an open camera.

        ``wait`` is the max seconds to wait for a frame (server-side; default 5).
        ``timeout`` is the gRPC call deadline (default 30 s).
        """
        req = pb.CaptureFrameRequest(camera_id=camera_id)
        if wait is not None:
            req.timeout = wait
        return self.stub.CaptureFrame(req, timeout=self._t(timeout, LONG_TIMEOUT))

    def capture_frames(
        self,
        camera_ids: list[str],
        *,
        timeout: float | None = None,
        wait: float | None = None,
    ) -> pb.CaptureFramesResponse:
        """Capture the latest frame from multiple cameras in a single request.

        Each camera is read concurrently server-side, so the round-trip
        latency is bounded by the slowest camera rather than the sum. Intended
        for use cases such as pairing an RGB frame with its depth map where
        splitting the capture across multiple requests would hurt real-time
        performance.

        ``camera_ids`` may contain duplicates (de-duplicated server-side); the
        response preserves the order of first appearance. Per-camera failures
        are reported on each ``CapturedFrame`` (``error_code`` / ``error``)
        rather than raising — inspect them to detect partial failures. A
        ``camera_id`` that is not registered is still returned with an
        ``error_code`` of ``NotFound``.

        ``wait`` is the max seconds to wait for a frame, per camera
        (server-side; default 5). ``timeout`` is the gRPC call deadline.
        """
        req = pb.CaptureFramesRequest(camera_ids=list(camera_ids))
        if wait is not None:
            req.timeout = wait
        return self.stub.CaptureFrames(req, timeout=self._t(timeout, LONG_TIMEOUT))

    def watch_frames(
        self, camera_id: str, *, timeout: float | None = None
    ) -> Iterator[pb.VideoFrame]:
        """Stream frames continuously from an open camera.

        Yields ``VideoFrame`` messages at the camera's native frame rate. The
        camera must have been opened via :meth:`open_camera` first. The stream
        ends when iteration stops (closing the RPC) or the camera is closed.

        Frame-mode (``jpeg`` / ``png`` / ``bmp`` / ``tiff``): each message is
        one complete, independently decodable frame. Bitstream-mode (``h264``
        / ``h265``): each message is a chunk of the Annex B elementary stream
        — concatenate in ``sequence`` order to rebuild the bitstream.
        """
        call = self.stub.WatchFrames(pb.WatchFramesRequest(camera_id=camera_id), timeout=timeout)
        return iter(call)

    # -- helpers --------------------------------------------------------------

    def _t(self, timeout: float | None, default: float | None = None) -> float:
        if timeout is not None:
            return timeout
        return default if default is not None else self._default_timeout


__all__ = ["CameraClient", "CameraState", "state_name"]
