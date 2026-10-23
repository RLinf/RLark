package grpc

import (
	"context"

	grpcpkg "google.golang.org/grpc"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
)

// UnaryServerErrorInterceptor converts execution faults returned by a unary handler to gRPC status errors.
func UnaryServerErrorInterceptor(ctx context.Context, request any, info *grpcpkg.UnaryServerInfo, handler grpcpkg.UnaryHandler) (any, error) {
	response, err := handler(ctx, request)
	return response, fault.ToGRPC(err)
}

// UnaryClientErrorInterceptor converts unary gRPC status errors to execution faults.
func UnaryClientErrorInterceptor(ctx context.Context, method string, request, reply any, connection *grpcpkg.ClientConn, invoker grpcpkg.UnaryInvoker, options ...grpcpkg.CallOption) error {
	return fault.FromGRPC(invoker(ctx, method, request, reply, connection, options...))
}
