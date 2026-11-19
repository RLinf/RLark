"""End-to-end example: drive a robot and stream camera frames.

Run inside a task pod that has requested the ``rlinf.io/device`` resource (the
device plugin injects the sockets and CLIs). Adjust the robot / camera IDs to
match your node's config.

    pip install embodied-runtime
    python example.py
"""

from embodied_runtime import CameraClient, RobotClient
from embodied_runtime.camera import state_name as cam_state
from embodied_runtime.robot import state_name as robot_state


def main() -> None:
    # --- robot ---
    with RobotClient() as robot:
        robots = robot.list_robots().robots
        if not robots:
            raise SystemExit("no robots registered on this node")
        robot_id = robots[0].robot_id

        print(f"modes for {robot_id}:", [m.name for m in robot.list_modes(robot_id).modes])

        # preset mode with a param override
        robot.start_robot(robot_id, mode="impedance", args={"robot_ip": "172.16.0.2"})
        status = robot.get_robot_status(robot_id)
        print(f"{status.robot_id}: {status.mode} ({robot_state(status.state)})")
        print(f"  ROS_MASTER_URI={status.ros_master_uri}")

    # --- camera ---
    with CameraClient() as cam:
        cameras = cam.list_cameras().cameras
        if not cameras:
            raise SystemExit("no cameras registered on this node")
        camera_id = cameras[0].camera_id

        opened = cam.open_camera(camera_id, encoding="h264")
        print(f"{opened.camera_id}: {cam_state(opened.state)} encoding={opened.encoding}")

        # stream 10 frames then stop
        for i, frame in enumerate(cam.watch_frames(camera_id)):
            print(
                f"  frame {frame.sequence}: {frame.width}x{frame.height} "
                f"{frame.encoding} {len(frame.data)} bytes keyframe={frame.keyframe}"
            )
            if i >= 9:
                break

        cam.close_camera(camera_id)


if __name__ == "__main__":
    main()
