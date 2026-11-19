package main

import (
	"log"
	"sort"
	"strings"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// newClient dials the Unix socket and returns a RobotControllerClient.
func newClient(socketPath string) (pb.RobotControllerClient, *grpc.ClientConn) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("dial %s: %v", socketPath, err)
	}
	return pb.NewRobotControllerClient(conn), conn
}

// stateStr converts a RobotState enum to a human-readable string.
func stateStr(s pb.RobotState) string {
	switch s {
	case pb.RobotState_ROBOT_STATE_STARTING:
		return "starting"
	case pb.RobotState_ROBOT_STATE_RUNNING:
		return "running"
	case pb.RobotState_ROBOT_STATE_STOPPING:
		return "stopping"
	case pb.RobotState_ROBOT_STATE_STOPPED:
		return "stopped"
	case pb.RobotState_ROBOT_STATE_ERROR:
		return "error"
	default:
		return "unknown"
	}
}

// formatMap formats a map as compact key=val,key=val for table display.
func formatMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
