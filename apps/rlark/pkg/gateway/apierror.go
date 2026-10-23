package gateway

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Error codes are part of the public REST contract. They are stable machine
// readable identifiers: clients may branch on them, so existing codes must not
// be renamed or repurposed. Messages are human readable and may change.
const (
	CodeInvalidQueryParameter = "INVALID_QUERY_PARAMETER"
	CodeInvalidRequestBody    = "INVALID_REQUEST_BODY"
	CodeInvalidRequest        = "INVALID_REQUEST"
	CodeUnauthorized          = "UNAUTHORIZED"
	CodeForbidden             = "FORBIDDEN"
	CodeResourceNotFound      = "RESOURCE_NOT_FOUND"
	CodeResourceAlreadyExists = "RESOURCE_ALREADY_EXISTS"
	CodeResourceConflict      = "RESOURCE_CONFLICT"
	CodeResourceInvalid       = "RESOURCE_INVALID"
	CodeUpstreamUnavailable   = "UPSTREAM_UNAVAILABLE"
	CodeInternalError         = "INTERNAL_ERROR"
	CodeNotConfigured         = "NOT_CONFIGURED"
)

// requestIDHeader carries the correlation identifier between clients, gateway
// logs and error responses.
const requestIDHeader = "X-Request-Id"

// requestIDContextKey is the gin context key holding the current request ID.
const requestIDContextKey = "rlark.requestID"

// ErrorResponse is the single error body shape returned by every gateway
// endpoint.
//
// Code is a stable machine readable identifier. Reason mirrors the Kubernetes
// StatusReason when the failure originated from the management API and is empty
// otherwise. RequestID correlates the response with gateway logs. Details
// carries structured, non-sensitive context such as the offending parameter.
type ErrorResponse struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Reason    string         `json:"reason,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// RequestIDMiddleware adopts an inbound X-Request-Id or generates one, then
// echoes it back so that clients can correlate an error with gateway logs.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(requestIDContextKey, id)
		c.Writer.Header().Set(requestIDHeader, id)
		c.Next()
	}
}

// requestIDOf returns the correlation identifier for the current request, or an
// empty string when the request did not pass through RequestIDMiddleware.
func requestIDOf(c *gin.Context) string {
	value, ok := c.Get(requestIDContextKey)
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}

// respondError writes the unified error body and aborts the handler chain.
func respondError(c *gin.Context, status int, code, message string) {
	respondErrorWithDetails(c, status, code, message, nil)
}

// respondErrorWithDetails writes the unified error body with structured
// context. Details must never contain credentials or other sensitive values.
func respondErrorWithDetails(c *gin.Context, status int, code, message string, details map[string]any) {
	respondErrorFull(c, status, code, "", message, details)
}

func respondErrorFull(c *gin.Context, status int, code, reason, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Code:      code,
		Message:   message,
		Reason:    reason,
		RequestID: requestIDOf(c),
		Details:   details,
	})
}

// respondKubeError maps an error returned by the management API onto the
// unified error contract. message describes the attempted operation and is used
// as-is so that callers control the wording; the underlying Kubernetes reason is
// reported separately in Reason.
func respondKubeError(c *gin.Context, message string, err error) {
	status, code := kubeErrorStatus(err)
	respondErrorFull(c, status, code, string(apierrors.ReasonForError(err)), message, nil)
}

// kubeErrorStatus classifies a management API error into an HTTP status and a
// stable error code. Unrecognised errors are reported as internal errors rather
// than being silently mapped onto a success or a not-found response.
func kubeErrorStatus(err error) (int, string) {
	switch {
	case apierrors.IsNotFound(err):
		return http.StatusNotFound, CodeResourceNotFound
	case apierrors.IsAlreadyExists(err):
		return http.StatusConflict, CodeResourceAlreadyExists
	case apierrors.IsConflict(err):
		return http.StatusConflict, CodeResourceConflict
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return http.StatusBadRequest, CodeResourceInvalid
	case apierrors.IsUnauthorized(err):
		return http.StatusUnauthorized, CodeUnauthorized
	case apierrors.IsForbidden(err):
		return http.StatusForbidden, CodeForbidden
	case apierrors.IsServiceUnavailable(err), apierrors.IsTimeout(err), apierrors.IsTooManyRequests(err):
		return http.StatusServiceUnavailable, CodeUpstreamUnavailable
	}

	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) && statusErr.ErrStatus.Code >= 400 {
		if statusErr.ErrStatus.Reason == metav1.StatusReasonUnsupportedMediaType {
			return http.StatusUnsupportedMediaType, CodeInvalidRequestBody
		}
		return int(statusErr.ErrStatus.Code), CodeInternalError
	}
	return http.StatusInternalServerError, CodeInternalError
}
