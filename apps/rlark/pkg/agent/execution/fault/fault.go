package fault

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Code classifies an execution-boundary failure independently of transport.
type Code string

const (
	// InvalidArgument indicates malformed or invalid caller input.
	InvalidArgument Code = "InvalidArgument"
	// NotFound indicates that the requested object does not exist.
	NotFound Code = "NotFound"
	// FailedPrecondition indicates that required state or capability is absent.
	FailedPrecondition Code = "FailedPrecondition"
	// AlreadyExists indicates that creation conflicts with an existing object.
	AlreadyExists Code = "AlreadyExists"
	// PermissionDenied indicates an authenticated caller lacks authorization.
	PermissionDenied Code = "PermissionDenied"
	// Unauthenticated indicates that caller identity could not be established.
	Unauthenticated Code = "Unauthenticated"
	// Conflict indicates an ownership, identity, or concurrent-state conflict.
	Conflict Code = "Conflict"
	// Unavailable indicates a retryable dependency outage.
	Unavailable Code = "Unavailable"
	// DeadlineExceeded indicates cancellation or timeout before completion.
	DeadlineExceeded Code = "DeadlineExceeded"
	// Internal indicates an unexpected implementation or invariant failure.
	Internal Code = "Internal"
)

// Error is a typed execution fault that preserves its underlying cause.
type Error struct {
	Code    Code
	Message string
	Cause   error
}

// Error returns the fault message and, when present, its cause text.
func (e *Error) Error() string {
	message := e.Message
	if message == "" {
		message = string(e.Code)
	}
	if e.Cause != nil {
		return message + ": " + e.Cause.Error()
	}
	return message
}

// Unwrap returns the underlying cause for errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// Is compares execution faults by Code.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && e.Code == other.Code
}

// New constructs an execution fault without an underlying cause.
func New(code Code, message string) error { return &Error{Code: code, Message: message} }

// Wrap constructs an execution fault while preserving the cause chain.
func Wrap(code Code, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// Is reports whether err contains an execution fault with the given code.
func Is(err error, code Code) bool { return errors.Is(err, &Error{Code: code}) }

// ToGRPC converts a local execution fault to its stable gRPC status code.
func ToGRPC(err error) error {
	if err == nil {
		return nil
	}
	if grpcStatus, ok := status.FromError(err); ok && grpcStatus.Code() != codes.Unknown {
		return err
	}
	var domain *Error
	if errors.As(err, &domain) {
		return status.Error(toGRPCCode(domain.Code), domain.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

// FromGRPC converts a gRPC status into a local execution fault and preserves err.
func FromGRPC(err error) error {
	if err == nil {
		return nil
	}
	grpcStatus, ok := status.FromError(err)
	if !ok {
		return err
	}
	return &Error{Code: fromGRPCCode(grpcStatus.Code()), Message: grpcStatus.Message(), Cause: err}
}

func toGRPCCode(code Code) codes.Code {
	switch code {
	case InvalidArgument:
		return codes.InvalidArgument
	case NotFound:
		return codes.NotFound
	case FailedPrecondition:
		return codes.FailedPrecondition
	case AlreadyExists:
		return codes.AlreadyExists
	case PermissionDenied:
		return codes.PermissionDenied
	case Unauthenticated:
		return codes.Unauthenticated
	case Conflict:
		return codes.Aborted
	case Unavailable:
		return codes.Unavailable
	case DeadlineExceeded:
		return codes.DeadlineExceeded
	case Internal:
		return codes.Internal
	default:
		panic(fmt.Sprintf("unknown execution fault code %q", code))
	}
}

func fromGRPCCode(code codes.Code) Code {
	switch code {
	case codes.InvalidArgument:
		return InvalidArgument
	case codes.NotFound:
		return NotFound
	case codes.FailedPrecondition:
		return FailedPrecondition
	case codes.AlreadyExists:
		return AlreadyExists
	case codes.PermissionDenied:
		return PermissionDenied
	case codes.Unauthenticated:
		return Unauthenticated
	case codes.Aborted:
		return Conflict
	case codes.Unavailable, codes.ResourceExhausted:
		return Unavailable
	case codes.DeadlineExceeded, codes.Canceled:
		return DeadlineExceeded
	default:
		return Internal
	}
}
