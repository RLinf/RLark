package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/zapr"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestARobotIsHeldByOneRunAtATimeUntilItIsReleasedOrItsLeaseIsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-robot", Name: "s1u-03-x86", ResourceVersion: "1"}}
	// The management API as the claim needs it: a merge patch that names a version other than the Node's is refused.
	var meanwhile func()
	g := &Gateway{accessors: map[string]*resourceAccessor{"nodes": {
		get: func(context.Context, string, string, metav1.GetOptions) (any, error) { return node.DeepCopy(), nil },
		list: func(context.Context, string, metav1.ListOptions) (any, error) {
			list := &rlarkv1alpha1.NodeList{}
			if node.Labels[robotClaimLabel] != "" {
				list.Items = append(list.Items, *node.DeepCopy())
			}
			return list, nil
		},
		patch: func(_ context.Context, _, _ string, data []byte) (any, error) {
			if first := meanwhile; first != nil {
				meanwhile = nil
				first()
			}
			var patch struct {
				Metadata struct {
					ResourceVersion     string
					Labels, Annotations map[string]*string
				}
			}
			_ = json.Unmarshal(data, &patch)
			if patch.Metadata.ResourceVersion != "" && patch.Metadata.ResourceVersion != node.ResourceVersion {
				return nil, apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, node.Name, errors.New("the object has been modified"))
			}
			for target, changes := range map[*map[string]string]map[string]*string{&node.Labels: patch.Metadata.Labels, &node.Annotations: patch.Metadata.Annotations} {
				if *target == nil {
					*target = map[string]string{}
				}
				for key, value := range changes {
					if value == nil {
						delete(*target, key)
					} else {
						(*target)[key] = *value
					}
				}
			}
			node.ResourceVersion += "1"
			return node, nil
		},
		update: func(context.Context, string, string, json.RawMessage) (any, error) { return node, nil },
	}}}
	core, logged := observer.New(zapcore.InfoLevel)
	ctx := log.WithLogger(context.Background(), zapr.NewLogger(zap.New(core)))
	router := gin.New()
	nodes := router.Group("/nodes", func(c *gin.Context) {
		c.Set(authSubjectKey, c.GetHeader("X-Subject"))
		c.Set(authRoleKey, c.GetHeader("X-Role"))
	})
	g.registerRobotRoutes(nodes)
	nodes.PATCH("/:name", g.handleKubePatch("nodes"))
	nodes.PUT("/:name", g.handleKubeUpdate("nodes"))
	call := func(method, path, subject, role, body string) (int, map[string]any) {
		request := httptest.NewRequest(method, "/nodes/s1u-03-x86"+path, strings.NewReader(body)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Subject", subject)
		request.Header.Set("X-Role", role)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var answer map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
		return recorder.Code, answer
	}
	claim := func(subject, body string) (int, map[string]any) {
		return call(http.MethodPut, "/robot-claim?namespace=rlark-robot", subject, "user", body)
	}
	stored := func() (claim robotClaim) {
		_ = json.Unmarshal([]byte(node.Annotations[robotClaimSetAnnotation]), &claim)
		return claim
	}
	// age makes the claim as it would be this much later.
	age := func(by time.Duration) {
		claim := stored()
		claim.GrantedAt, claim.Until = claim.GrantedAt.Add(-by), claim.Until.Add(-by)
		text, _ := json.Marshal(claim)
		node.Annotations[robotClaimSetAnnotation] = string(text)
	}
	audit := func() map[string]any {
		entries := logged.FilterMessage("robot claim").All()
		return entries[len(entries)-1].ContextMap()
	}
	holderOf := func(answer map[string]any) any {
		return answer["details"].(map[string]any)["current"].(map[string]any)["holder"]
	}

	// A free robot is the applicant's, for a minute unless it says otherwise; who applied is the caller.
	status, answer := claim("service-account:fleet", `{"holder":"run-a","job":"job-1","by":"mallory"}`)
	if granted := stored(); status != http.StatusOK || answer["holder"] != "run-a" || answer["by"] != "service-account:fleet" ||
		node.Labels[robotClaimLabel] != "run-a" || granted.Job != "job-1" || granted.Until.Sub(granted.GrantedAt) != time.Minute {
		t.Fatalf("grant: %d %v, %v", status, answer, node.ObjectMeta)
	}
	// Anyone else is told who holds it: another holder, and the same holder's name from another caller.
	for _, other := range [][2]string{{"bob", "run-b"}, {"bob", "run-a"}} {
		if status, answer := claim(other[0], `{"holder":"`+other[1]+`"}`); status != http.StatusConflict || answer["code"] != CodeRobotClaimBusy || holderOf(answer) != "run-a" {
			t.Fatalf("held by another, %v applies: %d %v", other, status, answer)
		}
	}
	for _, body := range []string{`{"holder":"not a label"}`, `{"holder":"run-b","job":"` + strings.Repeat("j", 254) + `"}`} {
		if status, _ := claim("bob", body); status != http.StatusBadRequest {
			t.Fatalf("a holder that is no label value, a job longer than a name: %d", status)
		}
	}
	// The holder applying again keeps the robot longer, from when it got it.
	age(30 * time.Second)
	before := stored()
	if status, _ := claim("service-account:fleet", `{"holder":"run-a","leaseSeconds":120}`); status != http.StatusOK ||
		!stored().GrantedAt.Equal(before.GrantedAt) || stored().Until.Sub(before.Until) < 90*time.Second {
		t.Fatalf("renew: %d, %v then %v", status, before, stored())
	}
	// Renewing only (?renew=true, any spelling of a boolean, Python's True included) keeps the holder's claim from
	// when it was granted, also one that is over and not swept yet; anyone else's is told who holds the robot. A renew
	// that is not a boolean is refused, never taken for a claim.
	renew := func(subject, holder string) (int, map[string]any) {
		return call(http.MethodPut, "/robot-claim?namespace=rlark-robot&renew=True", subject, "user", `{"holder":"`+holder+`"}`)
	}
	if status, _ := call(http.MethodPut, "/robot-claim?namespace=rlark-robot&renew=yes", "service-account:fleet", "user", `{"holder":"run-a"}`); status != http.StatusBadRequest {
		t.Fatalf("renew=yes: %d", status)
	}
	age(10 * time.Minute)
	granted := stored().GrantedAt
	if status, _ := renew("service-account:fleet", "run-a"); status != http.StatusOK || !stored().GrantedAt.Equal(granted) || !time.Now().Before(stored().Until) {
		t.Fatalf("renewal of a claim that is over: %d, %v", status, stored())
	}
	if status, answer := renew("bob", "run-a"); status != http.StatusConflict || holderOf(answer) != "run-a" {
		t.Fatalf("renewal by another caller: %d %v", status, answer)
	}
	// A claim whose lease is over is still the robot's until the sweeper ends it: nobody else gets it meanwhile.
	age(10 * time.Minute)
	if status, answer := claim("carol", `{"holder":"run-c"}`); status != http.StatusConflict || holderOf(answer) != "run-a" {
		t.Fatalf("a claim over and not swept, applied for by another: %d %v", status, answer)
	}
	if !g.expireRobotClaims(ctx, true) || len(node.Labels) != 0 {
		t.Fatalf("sweep of a claim that is over: %v", node.Labels)
	}
	// Of two who apply at once one gets the robot: the other's write is on a Node that changed, and it is told who
	// holds the robot now.
	meanwhile = func() {
		if status, _ := claim("carol", `{"holder":"run-c"}`); status != http.StatusOK {
			t.Fatalf("the first of two applicants: %d", status)
		}
		if line := audit(); line["actor"] != "carol" || line["holder"] != "run-c" || line["renewed"] != false {
			t.Fatalf("the grant's audit line: %v", line)
		}
	}
	if status, answer := claim("bob", `{"holder":"run-b"}`); status != http.StatusConflict || holderOf(answer) != "run-c" || node.Labels[robotClaimLabel] != "run-c" {
		t.Fatalf("the second of two applicants: %d %v", status, answer)
	}
	// Only the holder gives the robot back; the Node's own PATCH and PUT do not touch the claim.
	if status, _ := call(http.MethodDelete, "/robot-claim?namespace=rlark-robot&holder=run-c", "bob", "user", ""); status != http.StatusConflict {
		t.Fatalf("release by another: %d", status)
	}
	if status, _ := call(http.MethodPatch, "", "admin", roleAdmin, `{"metadata":{"labels":{"rlark.io/robot-claim":null}}}`); status != http.StatusBadRequest || node.Labels[robotClaimLabel] != "run-c" {
		t.Fatalf("PATCH of the claim: %d", status)
	}
	for _, labels := range []string{`{}`, `{"rlark.io/robot-claim":"run-b"}`} {
		if status, _ := call(http.MethodPut, "", "admin", roleAdmin, `{"metadata":{"labels":`+labels+`}}`); status != http.StatusBadRequest {
			t.Fatalf("PUT with the labels %s: %d", labels, status)
		}
	}
	// The release answers the claim it ended, {} when there was none; a renewal of a claim that is gone grants nothing.
	if status, answer := call(http.MethodDelete, "/robot-claim?namespace=rlark-robot&holder=run-c", "carol", "user", ""); status != http.StatusOK ||
		answer["holder"] != "run-c" || answer["by"] != "carol" || len(node.Labels)+len(node.Annotations) != 0 {
		t.Fatalf("release: %d %v, %v", status, answer, node.ObjectMeta)
	}
	if status, answer := call(http.MethodDelete, "/robot-claim?namespace=rlark-robot&holder=run-c", "carol", "user", ""); status != http.StatusOK || len(answer) != 0 {
		t.Fatalf("release of no claim: %d %v", status, answer)
	}
	if status, answer := renew("carol", "run-c"); status != http.StatusConflict || answer["details"].(map[string]any)["current"] != nil || len(node.Labels) != 0 {
		t.Fatalf("renewal of no claim: %d %v, %v", status, answer, node.Labels)
	}
	// A forced release is an administrator's or an operator's, with a reason, and recorded with it.
	claim("service-account:fleet", `{"holder":"run-a"}`)
	force := "/robot-claim?namespace=rlark-robot&force=true"
	if status, _ := call(http.MethodDelete, force+"&reason=stuck", "bob", "user", ""); status != http.StatusForbidden {
		t.Fatalf("forced release by a user: %d", status)
	}
	if status, _ := call(http.MethodDelete, force, "olga", roleOperator, ""); status != http.StatusBadRequest || node.Labels[robotClaimLabel] != "run-a" {
		t.Fatalf("forced release without a reason: %d", status)
	}
	if status, answer := call(http.MethodDelete, force+"&reason=stuck", "olga", roleOperator, ""); status != http.StatusOK || answer["holder"] != "run-a" || len(node.Labels) != 0 {
		t.Fatalf("forced release: %d %v, %v", status, answer, node.Labels)
	}
	if line := audit(); line["actor"] != "olga" || line["forced"] != true || line["reason"] != "stuck" || line["holder"] != "run-a" || line["by"] != "service-account:fleet" {
		t.Fatalf("the forced release's audit line: %v", line)
	}
	// The sweeper ends a claim whose lease is over, and no other.
	claim("service-account:fleet", `{"holder":"run-a"}`)
	if !g.expireRobotClaims(ctx, true) || node.Labels[robotClaimLabel] != "run-a" {
		t.Fatal("a claim within its lease was swept")
	}
	age(61 * time.Second)
	// The first list after the upstream comes back ends nothing: the holders renew within the minute that follows.
	if !g.expireRobotClaims(ctx, false) || node.Labels[robotClaimLabel] != "run-a" {
		t.Fatal("a claim was swept by the first list after an outage")
	}
	g.expireRobotClaims(ctx, true)
	if line := audit(); len(node.Labels)+len(node.Annotations) != 0 || line["action"] != "expire" || line["holder"] != "run-a" {
		t.Fatalf("sweep: %v, %v", node.ObjectMeta, line)
	}
	// An operator sets a robot's desired power; a user does not.
	if status, _ := call(http.MethodPut, "/robot-power?namespace=rlark-robot", "bob", "user", `{"desired":"off"}`); status != http.StatusForbidden {
		t.Fatalf("power set by a user: %d", status)
	}
	if status, _ := call(http.MethodPut, "/robot-power?namespace=rlark-robot", "olga", roleOperator, `{"desired":"off"}`); status != http.StatusOK || !strings.HasPrefix(node.Labels[robotPowerLabel], "off.") {
		t.Fatalf("power set by an operator: %d", status)
	}
}
