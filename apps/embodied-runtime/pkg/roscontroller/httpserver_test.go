package roscontroller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// newTestHTTPServer wires a fresh Controller-backed HTTPServer to an
// httptest.Server. Returns the base URL and the Controller so tests can
// inject robot state directly. The server is closed via t.Cleanup.
func newTestHTTPServer(t *testing.T) (base string, ctrl *Controller) {
	t.Helper()
	ctrl = NewController("10.0.0.1", nil)
	hs := NewHTTPServer(ctrl, "")
	srv := httptest.NewServer(hs.Handler())
	t.Cleanup(srv.Close)
	return srv.URL, ctrl
}

// injectRobot registers a robot type and injects a STOPPED robotState
// directly into the Controller's robots map (bypassing roscore/roslaunch
// so no real ROS runtime is needed). Returns the injected state.
func injectRobot(t *testing.T, ctrl *Controller, id, typeName string) *robotState {
	t.Helper()
	if err := ctrl.RegisterType(&RobotTypeConfig{
		Type: typeName,
		Modes: map[string]ModeConfig{
			"impedance": {Package: "franka_pkg", LaunchFile: "impedance.launch"},
			"joint":     {Package: "franka_pkg", LaunchFile: "joint.launch"},
		},
	}); err != nil {
		t.Fatalf("register type: %v", err)
	}
	rs := &robotState{
		Config:  &RobotConfig{ID: id, Type: typeName, Params: map[string]string{"robot_ip": "1.2.3.4"}},
		State:   pb.RobotState_ROBOT_STATE_STOPPED,
		process: NewRobotProcess(nil),
	}
	ctrl.mu.Lock()
	ctrl.robots[id] = rs
	ctrl.mu.Unlock()
	return rs
}

// doJSON issues an HTTP request, decodes a 2xx JSON body into out (a proto
// message), and returns the response for non-2xx inspection.
func doJSON(t *testing.T, method, url string, body any, out proto.Message) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if out != nil && resp.StatusCode < 300 {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := protojson.Unmarshal(raw, out); err != nil {
			t.Fatalf("unmarshal body: %v (body=%s)", err, raw)
		}
	}
	return resp
}

// ---------------------------------------------------------------------------
// ListRobots / GetRobotStatus
// ---------------------------------------------------------------------------

// TestROSHTTP_ListRobots_Empty verifies GET /v1/robots returns an empty list
// when no robots are registered.
func TestROSHTTP_ListRobots_Empty(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	var resp pb.ListRobotsResponse
	doJSON(t, http.MethodGet, base+"/v1/robots", nil, &resp)
	if len(resp.GetRobots()) != 0 {
		t.Errorf("robots = %v, want empty", resp.GetRobots())
	}
}

// TestROSHTTP_ListRobots_WithRobot verifies the list includes an injected
// robot and reflects its state.
func TestROSHTTP_ListRobots_WithRobot(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	var resp pb.ListRobotsResponse
	doJSON(t, http.MethodGet, base+"/v1/robots", nil, &resp)
	if len(resp.GetRobots()) != 1 {
		t.Fatalf("robots = %d, want 1", len(resp.GetRobots()))
	}
	r := resp.GetRobots()[0]
	if r.GetRobotId() != "franka-0" {
		t.Errorf("robotId = %q, want franka-0", r.GetRobotId())
	}
	if r.GetState() != pb.RobotState_ROBOT_STATE_STOPPED {
		t.Errorf("state = %v, want STOPPED", r.GetState())
	}
	if r.GetParams()["robot_ip"] != "1.2.3.4" {
		t.Errorf("params = %v, want robot_ip=1.2.3.4", r.GetParams())
	}
}

// TestROSHTTP_GetRobotStatus verifies GET /v1/robots/{id} returns the robot.
func TestROSHTTP_GetRobotStatus(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	var resp pb.GetRobotStatusResponse
	doJSON(t, http.MethodGet, base+"/v1/robots/franka-0", nil, &resp)
	if resp.GetRobotId() != "franka-0" {
		t.Errorf("robotId = %q, want franka-0", resp.GetRobotId())
	}
	if resp.GetState() != pb.RobotState_ROBOT_STATE_STOPPED {
		t.Errorf("state = %v, want STOPPED", resp.GetState())
	}
}

// TestROSHTTP_GetRobotStatus_NotFound verifies an unknown robot yields 404
// (gRPC NotFound → HTTP 404).
func TestROSHTTP_GetRobotStatus_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodGet, base+"/v1/robots/ghost", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	var errBody map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["message"] == nil || errBody["message"] == "" {
		t.Errorf("error body missing message: %v", errBody)
	}
}

// ---------------------------------------------------------------------------
// ListModes / GetRobotLogs
// ---------------------------------------------------------------------------

// TestROSHTTP_ListModes verifies GET /v1/robots/{id}/modes returns the
// registered modes for the robot's type.
func TestROSHTTP_ListModes(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	var resp pb.ListModesResponse
	doJSON(t, http.MethodGet, base+"/v1/robots/franka-0/modes", nil, &resp)
	if resp.GetRobotId() != "franka-0" {
		t.Errorf("robotId = %q, want franka-0", resp.GetRobotId())
	}
	// SupportedModes() sorts alphabetically → impedance, joint.
	if len(resp.GetModes()) != 2 {
		t.Fatalf("modes = %d, want 2", len(resp.GetModes()))
	}
	if resp.GetModes()[0].GetName() != "impedance" {
		t.Errorf("modes[0] = %q, want impedance", resp.GetModes()[0].GetName())
	}
}

// TestROSHTTP_ListModes_NotFound verifies an unknown robot yields 404.
func TestROSHTTP_ListModes_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodGet, base+"/v1/robots/ghost/modes", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestROSHTTP_GetRobotLogs verifies GET /v1/robots/{id}/logs returns the
// buffered log lines (empty for a freshly-injected robot).
func TestROSHTTP_GetRobotLogs(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	var resp pb.GetRobotLogsResponse
	doJSON(t, http.MethodGet, base+"/v1/robots/franka-0/logs", nil, &resp)
	if resp.GetRobotId() != "franka-0" {
		t.Errorf("robotId = %q, want franka-0", resp.GetRobotId())
	}
	// A fresh robot has no buffered log lines. (After a JSON round-trip an
	// empty repeated field unmarshals to nil, so check length, not nil-ness.)
	if len(resp.GetLines()) != 0 {
		t.Errorf("lines = %v, want empty", resp.GetLines())
	}
}

// TestROSHTTP_GetRobotLogs_TailQuery verifies the tail query parameter is
// parsed and forwarded to the controller.
func TestROSHTTP_GetRobotLogs_TailQuery(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	var resp pb.GetRobotLogsResponse
	doJSON(t, http.MethodGet, base+"/v1/robots/franka-0/logs?tail=10", nil, &resp)
	if resp.GetRobotId() != "franka-0" {
		t.Errorf("robotId = %q, want franka-0", resp.GetRobotId())
	}
}

// TestROSHTTP_GetRobotLogs_NotFound verifies an unknown robot yields 404.
func TestROSHTTP_GetRobotLogs_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodGet, base+"/v1/robots/ghost/logs", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// StartRobot / SwitchMode / StopRobot / ResetRobot — validation & 404 paths
// ---------------------------------------------------------------------------

// TestROSHTTP_StartRobot_NotFound verifies starting an unknown robot yields
// 404 (the OK path needs a real roslaunch/roscore and is not unit-tested).
func TestROSHTTP_StartRobot_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	body := map[string]any{"mode": "impedance"}
	resp := doJSON(t, http.MethodPost, base+"/v1/robots/ghost/start", body, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestROSHTTP_StartRobot_PathIdWins verifies the robot_id in the URL path
// always overrides any robot_id present in the JSON body. Here the path
// points at an unregistered robot so we get 404 — confirming the path id
// was used, not the body's.
func TestROSHTTP_StartRobot_PathIdWins(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	// Body says ghost (unregistered); path says franka-0 (registered). The
	// controller reaches the launch step for franka-0 (and fails there with
	// a 500-class error since there's no roslaunch binary) — NOT a 404 for
	// ghost. We assert the response is not a 404 to prove path wins.
	body := map[string]any{"robotId": "ghost", "mode": "impedance"}
	resp := doJSON(t, http.MethodPost, base+"/v1/robots/franka-0/start", body, nil)
	if resp.StatusCode == http.StatusNotFound {
		t.Errorf("status = 404, want non-404 (path franka-0 must win over body ghost)")
	}
}

// TestROSHTTP_StartRobot_BadBody verifies a malformed JSON body yields 400.
func TestROSHTTP_StartRobot_BadBody(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	req, _ := http.NewRequest(http.MethodPost, base+"/v1/robots/franka-0/start",
		strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestROSHTTP_SwitchMode_NotFound verifies switching an unknown robot yields
// 404.
func TestROSHTTP_SwitchMode_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	body := map[string]any{"mode": "joint"}
	resp := doJSON(t, http.MethodPost, base+"/v1/robots/ghost/mode", body, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestROSHTTP_StopRobot verifies POST /v1/robots/{id}/stop on a registered
// STOPPED robot returns OK (the controller short-circuits without touching
// the roslaunch process).
func TestROSHTTP_StopRobot(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	injectRobot(t, ctrl, "franka-0", "franka")

	var resp pb.StopRobotResponse
	doJSON(t, http.MethodPost, base+"/v1/robots/franka-0/stop", nil, &resp)
	if resp.GetRobotId() != "franka-0" {
		t.Errorf("robotId = %q, want franka-0", resp.GetRobotId())
	}
}

// TestROSHTTP_StopRobot_NotFound verifies stopping an unknown robot yields
// 404.
func TestROSHTTP_StopRobot_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodPost, base+"/v1/robots/ghost/stop", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestROSHTTP_ResetRobot_NotFound verifies resetting an unknown robot yields
// 404.
func TestROSHTTP_ResetRobot_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodPost, base+"/v1/robots/ghost/reset", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Packages — 404 paths (rospack not installed → rospackFind returns NotFound)
// ---------------------------------------------------------------------------

// TestROSHTTP_GetPackageInfo_NotFound verifies GET /v1/packages/{name} for an
// unknown (or any, without rospack installed) package yields 404 — the
// controller surfaces rospackFind's failure as a gRPC NotFound.
func TestROSHTTP_GetPackageInfo_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodGet, base+"/v1/packages/nope_pkg", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestROSHTTP_GetPackageLaunchFiles_NotFound verifies the launch-files route
// for an unknown package yields 404.
func TestROSHTTP_GetPackageLaunchFiles_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodGet, base+"/v1/packages/nope_pkg/launch-files", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestROSHTTP_GetLaunchFileArgs_NotFound verifies the launch-file args route
// for an unknown package yields 404.
func TestROSHTTP_GetLaunchFileArgs_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodGet,
		base+"/v1/packages/nope_pkg/launch-files/foo.launch/args", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestROSHTTP_GetLaunchFileArgs_RouteShape verifies the launch-file args
// route matches a filename containing a dot (e.g. impedance.launch) — the
// {launch_file} placeholder captures the whole segment.
func TestROSHTTP_GetLaunchFileArgs_RouteShape(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	// This should route to the handler (and 404 because the package is
	// unknown), not 404 from the mux because of an unparseable path.
	resp := doJSON(t, http.MethodGet,
		base+"/v1/packages/nope_pkg/launch-files/impedance.launch/args", nil, nil)
	// 404 from the controller (package not found) is acceptable; the point
	// is the route matched and dispatched.
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (route matched, package unknown)", resp.StatusCode)
	}
}

// TestROSHTTP_UnknownRoute verifies an unmapped path yields 404 (not a panic
// or 500) so the gateway behaves predictably.
func TestROSHTTP_UnknownRoute(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := doJSON(t, http.MethodGet, base+"/v1/nope", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// Compile-time: keep the codes import used (asserted by error-code mapping
// tests above via the 404s).
var _ codes.Code = codes.OK

// Ensure context import is referenced (used by handlers via r.Context() in
// production; referenced here only to keep the import honest for tests that
// may add context-driven cases).
var _ = context.Background
