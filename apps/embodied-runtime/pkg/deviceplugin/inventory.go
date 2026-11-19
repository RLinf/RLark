package deviceplugin

import (
	"context"
	"log"
	"time"

	camerapb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	rospb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ---------------------------------------------------------------------------
// Controller inventory cache
// ---------------------------------------------------------------------------
//
// detectDevices periodically lists robots and cameras from the
// ros-controller, ros2-controller, and camera-controller via their Unix
// sockets and caches the results. The cached lists are exposed through
// Robots() / ROS2Robots() / Cameras() for other components (e.g. Allocate)
// to inspect the node-local device
// inventory without opening a new gRPC connection on every request.

// dialUnix dials a gRPC Unix socket and returns the connection. The caller
// must close the connection when done.
func dialUnix(socketPath string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// refreshRobotInventory lists robots from the ros-controller via its Unix
// socket and caches the result. Best-effort: on error the existing cache is
// left untouched and the error is logged.
func (p *Plugin) refreshRobotInventory(ctx context.Context) {
	conn, err := dialUnix(ROSCtrlSocketPath)
	if err != nil {
		log.Printf("[device-plugin] dial ros-controller: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := rospb.NewRobotControllerClient(conn).ListRobots(listCtx, &rospb.ListRobotsRequest{})
	if err != nil {
		log.Printf("[device-plugin] list robots: %v", err)
		return
	}

	p.mu.Lock()
	p.robots = resp.Robots
	p.mu.Unlock()
}

// refreshCameraInventory lists cameras from the camera-controller via its
// Unix socket and caches the result. Best-effort: on error the existing
// cache is left untouched and the error is logged.
func (p *Plugin) refreshCameraInventory(ctx context.Context) {
	conn, err := dialUnix(CameraCtrlSocketPath)
	if err != nil {
		log.Printf("[device-plugin] dial camera-controller: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := camerapb.NewCameraControllerClient(conn).ListCameras(listCtx, &camerapb.ListCamerasRequest{})
	if err != nil {
		log.Printf("[device-plugin] list cameras: %v", err)
		return
	}

	p.mu.Lock()
	p.cameras = resp.Cameras
	p.mu.Unlock()
}

// refreshROS2RobotInventory lists robots from the ros2-controller via its
// Unix socket and caches the result. Best-effort: on error the existing
// cache is left untouched and the error is logged.
func (p *Plugin) refreshROS2RobotInventory(ctx context.Context) {
	conn, err := dialUnix(ROS2CtrlSocketPath)
	if err != nil {
		log.Printf("[device-plugin] dial ros2-controller: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := rospb.NewRobotControllerClient(conn).ListRobots(listCtx, &rospb.ListRobotsRequest{})
	if err != nil {
		log.Printf("[device-plugin] list ros2 robots: %v", err)
		return
	}

	p.mu.Lock()
	p.ros2Robots = resp.Robots
	p.mu.Unlock()
}

// Robots returns a snapshot of the cached robot inventory. The returned
// slice is a copy and may be freely modified by the caller.
func (p *Plugin) Robots() []*rospb.RobotInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*rospb.RobotInfo, len(p.robots))
	copy(out, p.robots)
	return out
}

// Cameras returns a snapshot of the cached camera inventory. The returned
// slice is a copy and may be freely modified by the caller.
func (p *Plugin) Cameras() []*camerapb.CameraDescriptor {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*camerapb.CameraDescriptor, len(p.cameras))
	copy(out, p.cameras)
	return out
}

// ROS2Robots returns a snapshot of the cached ROS 2 robot inventory. The
// returned slice is a copy and may be freely modified by the caller.
func (p *Plugin) ROS2Robots() []*rospb.RobotInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*rospb.RobotInfo, len(p.ros2Robots))
	copy(out, p.ros2Robots)
	return out
}
