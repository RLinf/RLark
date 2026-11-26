package iam

import "testing"

func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		role         Role
		method, path string
		want         bool
	}{
		{RoleAdmin, "POST", "/api/v1/unknown", true},
		{RoleOperator, "GET", "/api/v1/rlinf.io/v1alpha1/jobs", true},
		{RoleOperator, "POST", "/api/v1/rlinf.io/v1alpha1/jobs", true},
		{RoleOperator, "GET", "/api/v1/rlinf.io/v1alpha1/pods/p1/terminal", true},
		{RoleOperator, "GET", "/api/v1/rlinf.io/v1alpha1/tasks/t1/tensorboard/index.html", true},
		{RoleOperator, "POST", "/api/v1/rlinf.io/v1alpha1/tasks/t1/tensorboard/index.html", false},
		{RoleViewer, "GET", "/api/v1/rlinf.io/v1alpha1/jobs", true},
		{RoleViewer, "DELETE", "/api/v1/rlinf.io/v1alpha1/jobs/j1", false},
		{RoleViewer, "GET", "/api/v1/rlinf.io/v1alpha1/pods/p1/terminal", false},
		{RoleOperator, "GET", "/api/v1/secrets", false},
		{RoleOperator, "GET", "/api/v1/service-accounts", false},
		{RoleOperator, "GET", "/api/v1/image-registries/abc/password", false},
		{RoleOperator, "GET", "/api/v1/certificates/agent", false},
		{RoleOperator, "GET", "/api/v1/station/node-joins", false},
		{RoleOperator, "GET", "/api/v1/users", false},
		{RoleOperator, "PUT", "/api/v1/system-config", false},
		{RoleOperator, "PUT", "/api/v1/rlinf.io/v1alpha1/nodes/r1/robot-power", true},
		{RoleOperator, "POST", "/api/v1/rlinf.io/v1alpha1/nodes/r1/robot-pose", true},
		{RoleViewer, "POST", "/api/v1/rlinf.io/v1alpha1/nodes/r1/robot-pose", false},
		{RoleOperator, "PUT", "/api/v1/rlinf.io/v1alpha1/nodes/r1/station", true},
		{RoleOperator, "PUT", "/api/v1/rlinf.io/v1alpha1/nodes/r1/station-mark", true},
		{RoleOperator, "GET", "/api/v1/rlinf.io/v1alpha1/nodes/r1/station-logs", true},
		{RoleOperator, "POST", "/api/v1/rlinf.io/v1alpha1/nodes/r1/robot-restart", true},
		{RoleOperator, "DELETE", "/api/v1/rlinf.io/v1alpha1/nodes/r1/station", false},
		{RoleViewer, "PUT", "/api/v1/rlinf.io/v1alpha1/nodes/r1/station", false},
		{RoleViewer, "POST", "/api/v1/rlinf.io/v1alpha1/nodes/r1/robot-restart", false},
		{RoleViewer, "GET", "/api/v1/rlinf.io/v1alpha1/nodes/r1/station-logs", false},
		{RoleOperator, "GET", "/api/v1/unknown", false},
		{RoleOperator, "GET", "/api/v1/rlinf.io/v1alpha1/jobs/j1/unknown", false},
		{RoleViewer, "GET", "/api/v1/rlinf.io/v1alpha1/pods/p1/unknown", false},
		{RoleOperator, "GET", "/api/v1/rlinf.io/v1alpha1/tasks/t1/tensorboard", false},
		{RoleOperator, "GET", "/api/v1/%2e%2e/secrets", false},
		{RoleOperator, "GET", "/api/v1/%2fsecrets", false},
		{RoleOperator, "GET", "/api/v1/a//b", false},
		{RoleOperator, "GET", "/api/v1/a/../b", false},
	}
	for _, tc := range cases {
		t.Run(string(tc.role)+" "+tc.method+" "+tc.path, func(t *testing.T) {
			if got := allowed(tc.role, tc.method, tc.path); got != tc.want {
				t.Fatalf("allowed()=%v, want %v", got, tc.want)
			}
		})
	}
}
