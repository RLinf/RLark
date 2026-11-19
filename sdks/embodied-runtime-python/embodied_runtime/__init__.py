"""Python SDK for the embodied-runtime gRPC services.

Provides ``RobotClient`` (ros.controller.v1.RobotController) and
``CameraClient`` (camera.controller.v1.CameraController) over Unix domain
sockets, mirroring the ``rosctr`` / ``camctr`` Go CLIs.

The ``RobotController`` service is implemented by both the ROS 1 controller
(``ros-controller`` on ``ros-ctrl.sock``) and the ROS 2 controller
(``ros2-controller`` on ``ros2-ctrl.sock``). The same ``RobotClient`` works
for either — pass the appropriate ``socket_path`` or let it auto-detect
from the ``RLINF_EMBODIED_ROS_SOCKET_PATH`` / ``RLINF_EMBODIED_ROS2_SOCKET_PATH``
env vars injected by the device plugin.
"""

from ._transport import (
    DEFAULT_CAMERA_SOCKET,
    DEFAULT_ROS_SOCKET,
    DEFAULT_ROS2_SOCKET,
)
from .camera import CameraClient, CameraState
from .robot import ModeConfig, RobotClient, RobotState

__all__ = [
    "RobotClient",
    "RobotState",
    "ModeConfig",
    "CameraClient",
    "CameraState",
    "DEFAULT_ROS_SOCKET",
    "DEFAULT_ROS2_SOCKET",
    "DEFAULT_CAMERA_SOCKET",
]

__version__ = "0.1.0"
