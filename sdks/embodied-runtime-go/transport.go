package embodiedruntime

import (
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Default Unix socket paths and dial timeouts used when connecting to the
// ROS and camera controllers.
const (
	DefaultROSSocket    = "/var/run/rlark/ros-ctrl.sock"
	DefaultROS2Socket   = "/var/run/rlark/ros2-ctrl.sock"
	DefaultCameraSocket = "/var/run/rlark/camera-ctrl.sock"
	DefaultTimeout      = 10 * time.Second
	LongTimeout         = 30 * time.Second
)

// UnixTarget returns the gRPC target string for a Unix domain socket path.
func UnixTarget(socketPath string) string {
	return "unix://" + socketPath
}

// Dial opens a gRPC client connection to target using insecure transport
// credentials.
func Dial(target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// ResolveTarget resolves the gRPC target for a controller. If address is set
// it is used directly; otherwise socketPath is used, falling back to
// defaultSocket when empty, and converted to a Unix target.
func ResolveTarget(socketPath, defaultSocket, address string) string {
	if address != "" {
		return address
	}
	path := socketPath
	if path == "" {
		path = defaultSocket
	}
	return UnixTarget(path)
}

// ROSSocketPath returns the Unix socket path for the ROS 1 controller. It
// reads RLINF_EMBODIED_ROS_SOCKET_PATH (injected by the device plugin when
// the ros-controller is enabled); falls back to the default path.
func ROSSocketPath() string {
	if v := os.Getenv("RLINF_EMBODIED_ROS_SOCKET_PATH"); v != "" {
		return v
	}
	return DefaultROSSocket
}

// ROS2SocketPath returns the Unix socket path for the ROS 2 controller. It
// reads RLINF_EMBODIED_ROS2_SOCKET_PATH (injected by the device plugin when
// the ros2-controller is enabled); falls back to the default path.
func ROS2SocketPath() string {
	if v := os.Getenv("RLINF_EMBODIED_ROS2_SOCKET_PATH"); v != "" {
		return v
	}
	return DefaultROS2Socket
}

// CameraSocketPath returns the Unix socket path for the camera controller.
// It reads RLINF_EMBODIED_CAMERA_SOCKET_PATH (injected by the device plugin
// when the camera-controller is enabled); falls back to the default path.
func CameraSocketPath() string {
	if v := os.Getenv("RLINF_EMBODIED_CAMERA_SOCKET_PATH"); v != "" {
		return v
	}
	return DefaultCameraSocket
}

// DialRobot dials the ROS 1 controller. The returned *grpc.ClientConn can be
// used with pb.NewRobotControllerClient — the same proto service is shared
// between ROS 1 and ROS 2 controllers.
func DialRobot(address string) (*grpc.ClientConn, error) {
	target := ResolveTarget(ROSSocketPath(), DefaultROSSocket, address)
	return Dial(target)
}

// DialRobot2 dials the ROS 2 controller. The returned *grpc.ClientConn can be
// used with pb.NewRobotControllerClient — the same proto service is shared
// between ROS 1 and ROS 2 controllers.
func DialRobot2(address string) (*grpc.ClientConn, error) {
	target := ResolveTarget(ROS2SocketPath(), DefaultROS2Socket, address)
	return Dial(target)
}

// DialCamera dials the camera controller. The returned *grpc.ClientConn can
// be used with pb.NewCameraControllerClient.
func DialCamera(address string) (*grpc.ClientConn, error) {
	target := ResolveTarget(CameraSocketPath(), DefaultCameraSocket, address)
	return Dial(target)
}
