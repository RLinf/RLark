package fault_test

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
)

func TestGRPCRoundTrip(t *testing.T) {
	tests := []struct {
		code fault.Code
		grpc codes.Code
	}{
		{fault.InvalidArgument, codes.InvalidArgument},
		{fault.NotFound, codes.NotFound},
		{fault.FailedPrecondition, codes.FailedPrecondition},
		{fault.AlreadyExists, codes.AlreadyExists},
		{fault.PermissionDenied, codes.PermissionDenied},
		{fault.Unauthenticated, codes.Unauthenticated},
		{fault.Conflict, codes.Aborted},
		{fault.Unavailable, codes.Unavailable},
		{fault.DeadlineExceeded, codes.DeadlineExceeded},
		{fault.Internal, codes.Internal},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			wire := fault.ToGRPC(fault.New(tt.code, "message"))
			if status.Code(wire) != tt.grpc {
				t.Fatalf("gRPC code = %v, want %v", status.Code(wire), tt.grpc)
			}
			if restored := fault.FromGRPC(wire); !fault.Is(restored, tt.code) {
				t.Fatalf("restored error = %v", restored)
			}
		})
	}
}

func TestUnknownErrorsBecomeInternal(t *testing.T) {
	wire := fault.ToGRPC(errors.New("boom"))
	if status.Code(wire) != codes.Internal || !fault.Is(fault.FromGRPC(wire), fault.Internal) {
		t.Fatalf("unexpected error mapping: %v", wire)
	}
}

func TestNilAndExistingStatus(t *testing.T) {
	if fault.ToGRPC(nil) != nil || fault.FromGRPC(nil) != nil {
		t.Fatal("nil error was not preserved")
	}
	wire := status.Error(codes.NotFound, "missing")
	if fault.ToGRPC(wire) != wire {
		t.Fatal("existing status was not preserved")
	}
}

func TestWrappedErrorIncludesCauseAndPreservesChain(t *testing.T) {
	cause := errors.New("configmap already exists")
	err := fault.Wrap(fault.AlreadyExists, "create Kubernetes object", cause)
	if got, want := err.Error(), "create Kubernetes object: configmap already exists"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) || !fault.Is(err, fault.AlreadyExists) {
		t.Fatalf("wrapped error does not preserve cause and code: %v", err)
	}
}
