package httpproto

import (
	"encoding/json"
	"io"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Marshal is the canonical proto JSON marshaler used by HTTP gateways.
// EmitUnpopulated ensures responses have a stable shape matching CLI output.
var Marshal = protojson.MarshalOptions{
	EmitUnpopulated: true,
}

// Unmarshal is the canonical proto JSON unmarshaler for request bodies.
// Unknown fields are discarded for forward-compatibility; both
// lowerCamelCase (canonical) and snake_case (proto field names) are
// accepted.
var Unmarshal = protojson.UnmarshalOptions{
	DiscardUnknown: true,
}

// WriteProto serializes a proto.Message as canonical proto JSON with the
// given HTTP status code.
func WriteProto(w http.ResponseWriter, code int, msg proto.Message) {
	b, err := Marshal.Marshal(msg)
	if err != nil {
		http.Error(w, "marshal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

// WriteError maps a gRPC status error to the closest HTTP status code and
// writes a JSON error object: {"code","message","status"}. Non-status
// errors fall back to a plain text body with HTTP 500.
func WriteError(w http.ResponseWriter, err error) {
	code := GRPCStatusToHTTP(err)
	st, ok := status.FromError(err)
	if !ok {
		http.Error(w, err.Error(), code)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"code":    int(st.Code()),
		"status":  http.StatusText(code),
		"message": st.Message(),
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}

// DecodeBody reads the JSON request body into the given proto message.
// Path-derived identifiers (robot_id etc.) should be set by the caller
// AFTER this call so a value in the JSON body can never override the path.
func DecodeBody(r *http.Request, req proto.Message) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "read body: %v", err)
	}
	defer func() { _ = r.Body.Close() }()
	if len(raw) > 0 {
		if err := Unmarshal.Unmarshal(raw, req); err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid request body: %v", err)
		}
	}
	return nil
}

// GRPCStatusToHTTP maps a gRPC status code to the closest HTTP status code.
// Non-status errors default to 500.
func GRPCStatusToHTTP(err error) int {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}
	switch st.Code() {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // nginx "Client Closed Request"
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default: // Unauthenticated, etc.
		return http.StatusUnauthorized
	}
}
