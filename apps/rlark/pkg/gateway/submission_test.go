package gateway

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/submission"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

func TestWorkloadCreateRecordsVerifiedSubmitter(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, foreignKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	const jobs = "/api/v1/rlinf.io/v1alpha1/jobs"
	const workflows = "/api/v1/rlinf.io/v1alpha1/workflows"
	for _, tc := range []struct {
		name, subject, account, signedPath, username, id string
		signingKey                                       ed25519.PrivateKey
		offset                                           time.Duration
		status                                           int
	}{
		{name: "IAM admin", subject: "service-account:astribot-iam-admin", account: "astribot-iam-admin", signedPath: jobs, signingKey: private, username: "alice", id: "id-alice", status: 201},
		{name: "IAM operator", subject: "service-account:astribot-iam-operator", account: "astribot-iam-operator", signedPath: jobs, signingKey: private, username: "alice", id: "id-alice", status: 201},
		{name: "native user", subject: "bob", username: "bob", status: 201},
		{name: "legacy IAM", subject: "service-account:astribot-iam-operator", status: 201},
		{name: "machine client", subject: "service-account:fleet", status: 201},
		{name: "wrong signer", subject: "service-account:astribot-iam-operator", account: "astribot-iam-operator", signedPath: jobs, signingKey: foreignKey, status: 401},
		{name: "wrong route", subject: "service-account:astribot-iam-operator", account: "astribot-iam-operator", signedPath: workflows, signingKey: private, status: 401},
		{name: "wrong account", subject: "service-account:astribot-iam-operator", account: "astribot-iam-admin", signedPath: jobs, signingKey: private, status: 401},
		{name: "expired", subject: "service-account:astribot-iam-operator", account: "astribot-iam-operator", signedPath: jobs, signingKey: private, offset: -time.Minute, status: 401},
		{name: "direct caller supplying assertion", subject: "bob", account: "astribot-iam-operator", signedPath: jobs, signingKey: private, status: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &Gateway{kubeClient: clientset.NewSimpleClientset(), submissionKey: public, tenants: tenant.StaticProvider{}, accessors: map[string]*resourceAccessor{
				"jobs": {create: func(_ context.Context, _ string, body json.RawMessage) (any, error) {
					var job rlarkiov1alpha1.Job
					return &job, json.Unmarshal(body, &job)
				}},
			}}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(authSubjectKey, tc.subject)
			c.Set(authRoleKey, roleAdmin)
			c.Request = httptest.NewRequest(http.MethodPost, jobs, strings.NewReader(`{"metadata":{"name":"example","annotations":{"rlark.io/submitted-by":"untrusted","rlark.io/submitter-id":"untrusted"}}}`))
			if tc.signingKey != nil {
				raw, err := submission.Sign(tc.signingKey, "id-alice", "alice", tc.account, tc.signedPath, time.Now().Add(tc.offset))
				require.NoError(t, err)
				c.Request.Header.Set(submission.Header, raw)
			}
			g.handleKubeCreate("jobs")(c)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status != http.StatusCreated {
				return
			}
			var job rlarkiov1alpha1.Job
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &job))
			require.Equal(t, tc.username, job.Annotations[tenant.SubmittedByAnnotation])
			require.Equal(t, tc.id, job.Annotations[tenant.SubmitterIDAnnotation])
			require.Equal(t, tc.subject, job.Annotations[tenant.CreatedByAnnotation])
		})
	}
}

func TestWorkloadReplacePreservesOriginalSubmitter(t *testing.T) {
	existing := &metav1.ObjectMeta{Annotations: map[string]string{
		tenant.CreatedByAnnotation:   "service-account:astribot-iam-operator",
		tenant.SubmittedByAnnotation: "alice",
		tenant.SubmitterIDAnnotation: "original-id",
	}}
	body, err := preserveTenantIdentity([]byte(`{"metadata":{"annotations":{"rlark.io/submitted-by":"bob","rlark.io/submitter-id":"new-id","rlark.io/restarted-at":"2026-11-26T00:00:00Z"}}}`), existing)
	require.NoError(t, err)
	var job rlarkiov1alpha1.Job
	require.NoError(t, json.Unmarshal(body, &job))
	require.Equal(t, "alice", job.Annotations[tenant.SubmittedByAnnotation])
	require.Equal(t, "original-id", job.Annotations[tenant.SubmitterIDAnnotation])
	require.NotEmpty(t, job.Annotations["rlark.io/restarted-at"])
}

func TestWorkloadPatchCannotReplaceOrRemoveSubmitter(t *testing.T) {
	var applied []byte
	g := &Gateway{accessors: map[string]*resourceAccessor{
		"jobs": {
			get: func(_ context.Context, _, _ string, _ metav1.GetOptions) (any, error) {
				return &rlarkiov1alpha1.Job{}, nil
			},
			patch: func(_ context.Context, _, _ string, data []byte) (any, error) {
				applied = data
				return &rlarkiov1alpha1.Job{}, nil
			},
		},
	}}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"metadata":{"annotations":{"rlark.io/submitted-by":"bob","rlark.io/submitter-id":null,"custom":"preserved"}}}`, http.StatusOK},
		{`{"metadata":{"annotations":null}}`, http.StatusBadRequest},
		{`{"metadata":null}`, http.StatusBadRequest},
	} {
		applied = nil
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(authRoleKey, roleAdmin)
		c.Params = gin.Params{{Key: "name", Value: "example"}}
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/rlinf.io/v1alpha1/jobs/example", strings.NewReader(tc.body))
		g.handleKubePatch("jobs")(c)
		require.Equal(t, tc.status, w.Code, w.Body.String())
		if tc.status == http.StatusOK {
			require.JSONEq(t, `{"metadata":{"annotations":{"custom":"preserved"}}}`, string(applied))
		} else {
			require.Nil(t, applied)
		}
	}
}
