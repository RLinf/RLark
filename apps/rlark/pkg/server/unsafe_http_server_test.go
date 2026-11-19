package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleHealthCheckUsesPeerBroadcastState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{}

	checkStatus := func(want int) {
		t.Helper()
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
		server.handleHealthCheck(ctx)
		if response.Code != want {
			t.Fatalf("status = %d, want %d", response.Code, want)
		}
	}

	checkStatus(http.StatusServiceUnavailable)
	server.peerBroadcasted.Store(true)
	checkStatus(http.StatusOK)
}
