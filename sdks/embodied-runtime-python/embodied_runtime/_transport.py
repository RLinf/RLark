"""gRPC transport helpers for the embodied-runtime Python SDK.

Both controllers (ros / camera) are served over Unix domain sockets inside the
node-local socket directory ``/var/run/rlark``. This module centralises channel
construction so the client classes stay focused on their RPC surface.
"""

from __future__ import annotations

import os

import grpc

# Default socket paths — kept in sync with cmd/{rosctr,camctr}/main.go and the
# RLINF_EMBODIED_*_SOCKET_PATH env vars injected by the device plugin. The env
# var wins over the hard-coded default, mirroring the Go CLIs' cmp.Or behaviour.
DEFAULT_ROS_SOCKET = os.environ.get(
    "RLINF_EMBODIED_ROS_SOCKET_PATH", "/var/run/rlark/ros-ctrl.sock"
)
DEFAULT_ROS2_SOCKET = os.environ.get(
    "RLINF_EMBODIED_ROS2_SOCKET_PATH", "/var/run/rlark/ros2-ctrl.sock"
)
DEFAULT_CAMERA_SOCKET = os.environ.get(
    "RLINF_EMBODIED_CAMERA_SOCKET_PATH", "/var/run/rlark/camera-ctrl.sock"
)

# Per-RPC defaults mirroring the Go CLIs' context timeouts (seconds).
DEFAULT_TIMEOUT = 10.0
LONG_TIMEOUT = 30.0
STREAM_TIMEOUT = None  # streams are open-ended; cancelled by the caller


def unix_target(socket_path: str) -> str:
    """Build a gRPC target string for a Unix domain socket.

    Mirrors the Go clients which dial ``"unix://" + socketPath``.
    """
    return "unix://" + socket_path


def create_channel(target: str) -> grpc.Channel:
    """Open an insecure gRPC channel to ``target``.

    ``target`` is a raw gRPC target string. For Unix sockets pass the result of
    :func:`unix_target`; for remote TCP servers pass ``"host:port"``.
    """
    return grpc.insecure_channel(target)


def resolve_target(
    socket_path: str | None,
    default_socket: str,
    address: str | None = None,
) -> str:
    """Resolve a gRPC target from explicit args.

    ``address`` (e.g. ``"host:50051"``) takes precedence and is returned as-is
    for TCP connections. Otherwise a Unix-socket target is built from
    ``socket_path`` (falling back to the controller default).
    """
    if address:
        return address
    return unix_target(socket_path or default_socket)
