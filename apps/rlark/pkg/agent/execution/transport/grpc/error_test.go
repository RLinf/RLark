package grpc_test

import (
	"context"
	"testing"

	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executiongrpc "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/transport/grpc"
)

func TestUnaryServerErrorInterceptor(t *testing.T) {
	_, err := executiongrpc.UnaryServerErrorInterceptor(context.Background(), nil, nil, func(context.Context, any) (any, error) {
		return nil, fault.New(fault.InvalidArgument, "invalid")
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v", status.Code(err))
	}
}

func TestUnaryClientErrorInterceptor(t *testing.T) {
	err := executiongrpc.UnaryClientErrorInterceptor(context.Background(), "/test", nil, nil, nil, func(context.Context, string, any, any, *grpcpkg.ClientConn, ...grpcpkg.CallOption) error {
		return status.Error(codes.Unavailable, "offline")
	})
	if !fault.Is(err, fault.Unavailable) {
		t.Fatalf("error = %v", err)
	}
}
