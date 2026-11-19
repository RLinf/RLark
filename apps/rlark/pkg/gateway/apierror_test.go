package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func newErrorTestRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/probe", handler)
	return router
}

func doErrorRequest(t *testing.T, router *gin.Engine, inboundRequestID string) (*httptest.ResponseRecorder, ErrorResponse) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if inboundRequestID != "" {
		request.Header.Set(requestIDHeader, inboundRequestID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v (body=%s)", err, recorder.Body.String())
	}
	return recorder, body
}

func TestRequestIDMiddlewareAdoptsInboundHeader(t *testing.T) {
	router := newErrorTestRouter(func(c *gin.Context) {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "limit must be a positive integer")
	})

	recorder, body := doErrorRequest(t, router, "inbound-request-id")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := recorder.Header().Get(requestIDHeader); got != "inbound-request-id" {
		t.Fatalf("response header %s = %q, want %q", requestIDHeader, got, "inbound-request-id")
	}
	if body.RequestID != "inbound-request-id" {
		t.Fatalf("body requestId = %q, want %q", body.RequestID, "inbound-request-id")
	}
	if body.Code != CodeInvalidQueryParameter {
		t.Fatalf("body code = %q, want %q", body.Code, CodeInvalidQueryParameter)
	}
	if body.Message != "limit must be a positive integer" {
		t.Fatalf("body message = %q", body.Message)
	}
	if body.Reason != "" {
		t.Fatalf("body reason = %q, want empty", body.Reason)
	}
	if body.Details != nil {
		t.Fatalf("body details = %v, want nil", body.Details)
	}
}

func TestRequestIDMiddlewareGeneratesWhenAbsent(t *testing.T) {
	router := newErrorTestRouter(func(c *gin.Context) {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "boom")
	})

	recorder, body := doErrorRequest(t, router, "")

	header := recorder.Header().Get(requestIDHeader)
	if header == "" {
		t.Fatalf("response header %s is empty, want generated id", requestIDHeader)
	}
	if body.RequestID != header {
		t.Fatalf("body requestId = %q, want %q", body.RequestID, header)
	}
}

func TestRequestIDMiddlewareIgnoresBlankInboundHeader(t *testing.T) {
	router := newErrorTestRouter(func(c *gin.Context) {
		respondError(c, http.StatusForbidden, CodeForbidden, "denied")
	})

	recorder, body := doErrorRequest(t, router, "   ")

	if body.RequestID == "" || body.RequestID == "   " {
		t.Fatalf("body requestId = %q, want generated id", body.RequestID)
	}
	if got := recorder.Header().Get(requestIDHeader); got != body.RequestID {
		t.Fatalf("response header %s = %q, want %q", requestIDHeader, got, body.RequestID)
	}
}

func TestRespondErrorWithDetails(t *testing.T) {
	router := newErrorTestRouter(func(c *gin.Context) {
		respondErrorWithDetails(c, http.StatusBadRequest, CodeInvalidRequestBody, "invalid body", map[string]any{
			"field": "spec.replicas",
		})
	})

	recorder, body := doErrorRequest(t, router, "with-details")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if body.Details["field"] != "spec.replicas" {
		t.Fatalf("body details = %v", body.Details)
	}
}

func TestRespondErrorAbortsHandlerChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())

	reached := false
	router.GET("/probe",
		func(c *gin.Context) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "not found")
		},
		func(c *gin.Context) {
			reached = true
		},
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if reached {
		t.Fatal("subsequent handler ran, want aborted chain")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestRespondKubeErrorCarriesReason(t *testing.T) {
	groupResource := schema.GroupResource{Group: "rlinf.io", Resource: "jobs"}
	router := newErrorTestRouter(func(c *gin.Context) {
		respondKubeError(c, "get job: not found", apierrors.NewNotFound(groupResource, "jo-1"))
	})

	recorder, body := doErrorRequest(t, router, "kube-error")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if body.Code != CodeResourceNotFound {
		t.Fatalf("body code = %q, want %q", body.Code, CodeResourceNotFound)
	}
	if body.Reason != string(metav1.StatusReasonNotFound) {
		t.Fatalf("body reason = %q, want %q", body.Reason, metav1.StatusReasonNotFound)
	}
	if body.Message != "get job: not found" {
		t.Fatalf("body message = %q", body.Message)
	}
	if body.RequestID != "kube-error" {
		t.Fatalf("body requestId = %q", body.RequestID)
	}
}

func TestKubeErrorStatus(t *testing.T) {
	groupResource := schema.GroupResource{Group: "rlinf.io", Resource: "jobs"}
	groupKind := schema.GroupKind{Group: "rlinf.io", Kind: "Job"}

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "not found",
			err:        apierrors.NewNotFound(groupResource, "jo-1"),
			wantStatus: http.StatusNotFound,
			wantCode:   CodeResourceNotFound,
		},
		{
			name:       "already exists",
			err:        apierrors.NewAlreadyExists(groupResource, "jo-1"),
			wantStatus: http.StatusConflict,
			wantCode:   CodeResourceAlreadyExists,
		},
		{
			name:       "conflict",
			err:        apierrors.NewConflict(groupResource, "jo-1", nil),
			wantStatus: http.StatusConflict,
			wantCode:   CodeResourceConflict,
		},
		{
			name: "invalid",
			err: apierrors.NewInvalid(groupKind, "jo-1", field.ErrorList{
				field.Required(field.NewPath("spec", "tasks"), "at least one task is required"),
			}),
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeResourceInvalid,
		},
		{
			name:       "bad request",
			err:        apierrors.NewBadRequest("malformed field selector"),
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeResourceInvalid,
		},
		{
			name:       "unauthorized",
			err:        apierrors.NewUnauthorized("token expired"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   CodeUnauthorized,
		},
		{
			name:       "forbidden",
			err:        apierrors.NewForbidden(groupResource, "jo-1", errTestForbidden{}),
			wantStatus: http.StatusForbidden,
			wantCode:   CodeForbidden,
		},
		{
			name:       "service unavailable",
			err:        apierrors.NewServiceUnavailable("upstream down"),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   CodeUpstreamUnavailable,
		},
		{
			name:       "timeout",
			err:        apierrors.NewTimeoutError("request timed out", 1),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   CodeUpstreamUnavailable,
		},
		{
			name:       "too many requests",
			err:        apierrors.NewTooManyRequestsError("slow down"),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   CodeUpstreamUnavailable,
		},
		{
			name: "unsupported media type",
			err: &apierrors.StatusError{ErrStatus: metav1.Status{
				Status:  metav1.StatusFailure,
				Code:    http.StatusUnsupportedMediaType,
				Reason:  metav1.StatusReasonUnsupportedMediaType,
				Message: "application/xml is not supported",
			}},
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   CodeInvalidRequestBody,
		},
		{
			name:       "internal error",
			err:        apierrors.NewInternalError(errTestForbidden{}),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternalError,
		},
		{
			name:       "non kubernetes error",
			err:        errTestForbidden{},
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternalError,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, code := kubeErrorStatus(testCase.err)
			if status != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", status, testCase.wantStatus)
			}
			if code != testCase.wantCode {
				t.Fatalf("code = %q, want %q", code, testCase.wantCode)
			}
		})
	}
}

type errTestForbidden struct{}

func (errTestForbidden) Error() string { return "test error" }
