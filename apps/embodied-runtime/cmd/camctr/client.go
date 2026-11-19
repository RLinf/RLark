package main

import (
	"log"
	"strconv"
	"strings"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// newClient dials the Unix socket and returns a CameraControllerClient.
func newClient(socketPath string) (pb.CameraControllerClient, *grpc.ClientConn) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("dial %s: %v", socketPath, err)
	}
	return pb.NewCameraControllerClient(conn), conn
}

// stateStr converts a CameraState enum to a human-readable string.
func stateStr(s pb.CameraState) string {
	switch s {
	case pb.CameraState_CAMERA_STATE_CLOSED:
		return "closed"
	case pb.CameraState_CAMERA_STATE_OPEN:
		return "open"
	case pb.CameraState_CAMERA_STATE_ERROR:
		return "error"
	default:
		return "unknown"
	}
}

// joinInt32 returns the values as a comma-separated string.
func joinInt32(v []int32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.Itoa(int(x))
	}
	return strings.Join(parts, ", ")
}
