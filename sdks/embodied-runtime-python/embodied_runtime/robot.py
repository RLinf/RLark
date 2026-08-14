"""Client for ``ros.controller.v1.RobotController``.

Wraps the generated gRPC stub with a Pythonic, Unix-socket-aware API that
mirrors the ``rosctr`` CLI. Robot lifecycle (start / stop / switch mode) and
ROS package introspection are exposed as plain methods returning the protobuf
response messages.

The ``RobotController`` service is implemented by two independent controller
binaries that register on separate Unix sockets:

  - **ROS 1** (``ros-controller`` on ``ros-ctrl.sock``): starts a per-robot
    ``roscore``, fills ``ros_master_uri`` in responses.
  - **ROS 2** (``ros2-controller`` on ``ros2-ctrl.sock``): no master; assigns
    a per-robot ``ROS_DOMAIN_ID`` for DDS isolation, fills ``ros_domain_id``
    in responses.

This client works with either controller — pass the appropriate
``socket_path`` or let it auto-detect from the
``RLINF_EMBODIED_ROS_SOCKET_PATH`` / ``RLINF_EMBODIED_ROS2_SOCKET_PATH`` env
vars.
"""

from __future__ import annotations

import grpc

from . import _transport
from ._transport import DEFAULT_ROS_SOCKET, DEFAULT_TIMEOUT
from .gen.roscontroller.v1 import robot_pb2 as pb
from .gen.roscontroller.v1 import robot_pb2_grpc as pb_grpc

# Re-export the proto enum so callers can ``from embodied_runtime import RobotState``.
RobotState = pb.RobotState

# Human-readable names mirroring cmd/rosctr/client.go stateStr().
_STATE_NAMES = {
    RobotState.ROBOT_STATE_UNSPECIFIED: "unknown",
    RobotState.ROBOT_STATE_STARTING: "starting",
    RobotState.ROBOT_STATE_RUNNING: "running",
    RobotState.ROBOT_STATE_STOPPING: "stopping",
    RobotState.ROBOT_STATE_STOPPED: "stopped",
    RobotState.ROBOT_STATE_ERROR: "error",
}


def state_name(state: int) -> str:
    """Return a lowercase human-readable name for a ``RobotState`` value."""
    return _STATE_NAMES.get(state, "unknown")


class ModeConfig:
    """Builder for an ad-hoc (custom) robot control mode.

    Mirrors the protobuf ``ModeConfig`` message and the ``--package`` /
    ``--launch-file`` / ``--arg`` / ``--arg-from`` / ``--passthrough-robot-args``
    / ``--env`` flags of ``rosctr start``.

    Either ``passthrough_robot_args`` (identity merge of all robot params) or
    ``arg_from`` (explicit arg-name → param-name mapping) may be used — they are
    mutually exclusive on the server side.
    """

    def __init__(
        self,
        package: str,
        launch_file: str,
        args: dict[str, str] | None = None,
        arg_from: dict[str, str] | None = None,
        passthrough_robot_args: bool = False,
        env: dict[str, str] | None = None,
    ) -> None:
        if not package:
            raise ValueError("package is required for a custom ModeConfig")
        if not launch_file:
            raise ValueError("launch_file is required for a custom ModeConfig")
        if passthrough_robot_args and arg_from:
            raise ValueError("passthrough_robot_args and arg_from are mutually exclusive")
        self.package = package
        self.launch_file = launch_file
        self.args = dict(args) if args else {}
        self.arg_from = dict(arg_from) if arg_from else {}
        self.passthrough_robot_args = passthrough_robot_args
        self.env = dict(env) if env else {}

    def _to_proto(self) -> pb.ModeConfig:
        return pb.ModeConfig(
            package=self.package,
            launch_file=self.launch_file,
            args=self.args,
            arg_from=self.arg_from,
            passthrough_robot_args=self.passthrough_robot_args,
            env=self.env,
        )

    def __repr__(self) -> str:
        return (
            f"ModeConfig(package={self.package!r}, launch_file={self.launch_file!r}, "
            f"args={self.args!r}, arg_from={self.arg_from!r}, "
            f"passthrough_robot_args={self.passthrough_robot_args}, env={self.env!r})"
        )


def _resolve_mode_config(
    mode: str,
    mode_config: ModeConfig | None,
    args: dict[str, str] | None,
    env: dict[str, str] | None,
) -> pb.ModeConfig | None:
    """Build the protobuf ``ModeConfig`` for a start/switch request.

    Mirrors ``resolveModeConfig`` in cmd/rosctr/mode_flags.go:

    * preset mode (``mode`` set): only ``args`` / ``env`` overrides are allowed;
      ``mode_config`` (which carries package/launch_file) is rejected.
    * custom mode (``mode`` empty): ``mode_config`` with package + launch_file is
      required.

    Returns ``None`` when no override is needed (preset with no extras), in which
    case the caller should leave ``mode_config`` unset on the request.
    """
    if mode:
        if mode_config is not None:
            raise ValueError("mode_config (custom mode) cannot be combined with a preset mode name")
        if not args and not env:
            return None
        return pb.ModeConfig(args=dict(args or {}), env=dict(env or {}))

    if mode_config is None:
        raise ValueError("either a preset mode name or a custom ModeConfig must be provided")
    return mode_config._to_proto()


class RobotClient:
    """gRPC client for the ros-controller ``RobotController`` service.

    By default it connects to the node-local Unix socket
    ``/var/run/rlark/ros-ctrl.sock`` (matching ``rosctr``). Pass ``address`` to
    reach a remote TCP server instead.

    Example::

        from embodied_runtime import RobotClient, ModeConfig

        with RobotClient() as robot:
            robot.start_robot("franka-0", mode="impedance")
            for info in robot.list_robots().robots:
                print(info.robot_id, state_name(info.state))
    """

    def __init__(
        self,
        socket_path: str | None = None,
        *,
        address: str | None = None,
        timeout: float = DEFAULT_TIMEOUT,
    ) -> None:
        """Create a RobotClient.

        ``socket_path`` selects the controller socket (ROS 1 or ROS 2). When
        ``None``, the default is resolved from the
        ``RLINF_EMBODIED_ROS_SOCKET_PATH`` env var (ROS 1) or the hard-coded
        default ``/var/run/rlark/ros-ctrl.sock``. Pass the ROS 2 socket
        (``/var/run/rlark/ros2-ctrl.sock``) to target the ROS 2 controller.
        """
        self._target = _transport.resolve_target(socket_path, DEFAULT_ROS_SOCKET, address)
        self._default_timeout = timeout
        self._channel: grpc.Channel | None = None
        self._stub: pb_grpc.RobotControllerStub | None = None

    # -- connection management -------------------------------------------------

    def connect(self) -> RobotClient:
        """Open the gRPC channel and create the stub. Idempotent."""
        if self._channel is None:
            self._channel = _transport.create_channel(self._target)
            self._stub = pb_grpc.RobotControllerStub(self._channel)
        return self

    def close(self) -> None:
        """Close the underlying gRPC channel."""
        if self._channel is not None:
            self._channel.close()
            self._channel = None
            self._stub = None

    def __enter__(self) -> RobotClient:
        return self.connect()

    def __exit__(self, exc_type, exc, tb) -> None:
        self.close()

    @property
    def stub(self) -> pb_grpc.RobotControllerStub:
        """The raw gRPC stub for advanced / streaming use."""
        if self._stub is None:
            raise RuntimeError("client is not connected; call connect() or use 'with'")
        return self._stub

    # -- robot lifecycle ------------------------------------------------------

    def start_robot(
        self,
        robot_id: str,
        mode: str = "",
        *,
        mode_config: ModeConfig | None = None,
        args: dict[str, str] | None = None,
        env: dict[str, str] | None = None,
        timeout: float | None = None,
    ) -> pb.StartRobotResponse:
        """Start a robot node.

        ``mode`` selects a preset (e.g. ``"impedance"``); ``args`` / ``env`` add
        per-launch overrides on top of the preset. For an ad-hoc mode, pass a
        :class:`ModeConfig` instead of a mode name.
        """
        cfg = _resolve_mode_config(mode, mode_config, args, env)
        req = pb.StartRobotRequest(robot_id=robot_id, mode=mode, mode_config=cfg)
        return self.stub.StartRobot(req, timeout=self._t(timeout))

    def stop_robot(self, robot_id: str, *, timeout: float | None = None) -> pb.StopRobotResponse:
        """Stop a running robot node."""
        return self.stub.StopRobot(pb.StopRobotRequest(robot_id=robot_id), timeout=self._t(timeout))

    def get_robot_status(
        self, robot_id: str, *, timeout: float | None = None
    ) -> pb.GetRobotStatusResponse:
        """Return the current status of a robot node."""
        return self.stub.GetRobotStatus(
            pb.GetRobotStatusRequest(robot_id=robot_id), timeout=self._t(timeout)
        )

    def switch_mode(
        self,
        robot_id: str,
        mode: str = "",
        *,
        mode_config: ModeConfig | None = None,
        args: dict[str, str] | None = None,
        env: dict[str, str] | None = None,
        timeout: float | None = None,
    ) -> pb.SwitchModeResponse:
        """Change the control mode of a running robot node."""
        cfg = _resolve_mode_config(mode, mode_config, args, env)
        req = pb.SwitchModeRequest(robot_id=robot_id, mode=mode, mode_config=cfg)
        return self.stub.SwitchMode(req, timeout=self._t(timeout))

    def reset_robot(self, robot_id: str, *, timeout: float | None = None) -> pb.ResetRobotResponse:
        """Stop the robot, restart the ROS middleware, and reset state.

        For ROS 1: restarts ``roscore`` on the same port. For ROS 2: restarts
        the ``ros2 launch`` process (there is no master to restart). The
        ``ROS_DOMAIN_ID`` / ``ROS_MASTER_URI`` is preserved across reset.
        """
        return self.stub.ResetRobot(
            pb.ResetRobotRequest(robot_id=robot_id), timeout=self._t(timeout)
        )

    def list_robots(self, *, timeout: float | None = None) -> pb.ListRobotsResponse:
        """Return all managed robots and their status."""
        return self.stub.ListRobots(pb.ListRobotsRequest(), timeout=self._t(timeout))

    def list_modes(self, robot_id: str, *, timeout: float | None = None) -> pb.ListModesResponse:
        """Return the supported control modes for a robot."""
        return self.stub.ListModes(pb.ListModesRequest(robot_id=robot_id), timeout=self._t(timeout))

    def get_robot_logs(
        self, robot_id: str, tail: int = 0, *, timeout: float | None = None
    ) -> pb.GetRobotLogsResponse:
        """Return recent launch process log lines. ``tail`` = last N lines; 0 = all."""
        return self.stub.GetRobotLogs(
            pb.GetRobotLogsRequest(robot_id=robot_id, tail=tail), timeout=self._t(timeout)
        )

    # -- package introspection ------------------------------------------------

    def list_packages(self, *, timeout: float | None = None) -> pb.ListPackagesResponse:
        """List ROS packages available on the server (whitelist-filtered)."""
        return self.stub.ListPackages(pb.ListPackagesRequest(), timeout=self._t(timeout))

    def get_package_info(
        self, name: str, *, timeout: float | None = None
    ) -> pb.GetPackageInfoResponse:
        """Return metadata about a ROS package."""
        return self.stub.GetPackageInfo(
            pb.GetPackageInfoRequest(name=name), timeout=self._t(timeout)
        )

    def get_package_launch_files(
        self, name: str, *, timeout: float | None = None
    ) -> pb.GetPackageLaunchFilesResponse:
        """List launch files in a ROS package."""
        return self.stub.GetPackageLaunchFiles(
            pb.GetPackageLaunchFilesRequest(name=name), timeout=self._t(timeout)
        )

    def get_launch_file_args(
        self, package: str, launch_file: str, *, timeout: float | None = None
    ) -> pb.GetLaunchFileArgsResponse:
        """Return the arguments supported by a launch file."""
        return self.stub.GetLaunchFileArgs(
            pb.GetLaunchFileArgsRequest(package=package, launch_file=launch_file),
            timeout=self._t(timeout),
        )

    # -- helpers --------------------------------------------------------------

    def _t(self, timeout: float | None) -> float:
        return self._default_timeout if timeout is None else timeout


__all__ = ["RobotClient", "RobotState", "ModeConfig", "state_name"]
