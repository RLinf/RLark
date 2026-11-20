package stationidentity

import (
	"context"
	"errors"
	"slices"
	"testing"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestAuthenticateRequiresFreshPodIdentity(t *testing.T) {
	reg := Registration{ClusterID: "cluster", Namespace: "agent", ServiceAccount: "station", ServiceAccountUID: "sa-uid", Audience: "station-api"}
	for _, tc := range []struct {
		name        string
		change      func(*authv1.TokenReviewStatus)
		nodeUID     types.UID
		reviewError bool
		wantOK      bool
	}{
		{name: "valid", nodeUID: "node-uid", wantOK: true},
		{name: "unauthenticated", nodeUID: "node-uid", change: func(s *authv1.TokenReviewStatus) { s.Authenticated = false }},
		{name: "wrong audience", nodeUID: "node-uid", change: func(s *authv1.TokenReviewStatus) { s.Audiences = []string{"other"} }},
		{name: "wrong service account UID", nodeUID: "node-uid", change: func(s *authv1.TokenReviewStatus) { s.User.UID = "other" }},
		{name: "wrong pod UID", nodeUID: "node-uid", change: func(s *authv1.TokenReviewStatus) {
			s.User.Extra["authentication.kubernetes.io/pod-uid"] = authv1.ExtraValue{"old"}
		}},
		{name: "multiple pod names", nodeUID: "node-uid", change: func(s *authv1.TokenReviewStatus) {
			s.User.Extra["authentication.kubernetes.io/pod-name"] = authv1.ExtraValue{"pod", "other"}
		}},
		{name: "review failure", nodeUID: "node-uid", reviewError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "station", Namespace: "agent", UID: "sa-uid"}}, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "agent", UID: "pod-uid"}, Spec: corev1.PodSpec{ServiceAccountName: "station", NodeName: "node"}}, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", UID: tc.nodeUID}})
			client.PrependReactor("create", "tokenreviews", func(action ktesting.Action) (bool, runtime.Object, error) {
				if tc.reviewError {
					return true, nil, errors.New("unavailable")
				}
				req := action.(ktesting.CreateAction).GetObject().(*authv1.TokenReview)
				if len(req.Spec.Audiences) != 1 || req.Spec.Audiences[0] != reg.Audience {
					t.Fatal("incorrect requested audience")
				}
				status := authv1.TokenReviewStatus{Authenticated: true, Audiences: []string{"station-api"}, User: authv1.UserInfo{Username: "system:serviceaccount:agent:station", UID: "sa-uid", Extra: map[string]authv1.ExtraValue{"authentication.kubernetes.io/pod-name": {"pod"}, "authentication.kubernetes.io/pod-uid": {"pod-uid"}}}}
				if tc.change != nil {
					tc.change(&status)
				}
				return true, &authv1.TokenReview{Status: status}, nil
			})
			evidence, err := Authenticate(context.Background(), client, reg, "test-token")
			if err == nil && evidence.NodeUID != "node-uid" {
				t.Fatalf("the evidence is not of the Pod's Node: %+v", evidence)
			}
			if (err == nil) != tc.wantOK {
				t.Fatalf("error = %v, want success %v", err, tc.wantOK)
			}
		})
	}
}

func TestAuthenticateReadsTheNodesPressures(t *testing.T) {
	reg := Registration{ClusterID: "cluster", Namespace: "agent", ServiceAccount: "station", ServiceAccountUID: "sa-uid", Audience: "station-api"}
	client := fake.NewSimpleClientset(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "station", Namespace: "agent", UID: "sa-uid"}}, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "agent", UID: "pod-uid"}, Spec: corev1.PodSpec{ServiceAccountName: "station", NodeName: "node-a"}}, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue}, {Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse}, {Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
	}}})
	client.PrependReactor("create", "tokenreviews", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.TokenReview{Status: authv1.TokenReviewStatus{Authenticated: true, Audiences: []string{"station-api"}, User: authv1.UserInfo{Username: "system:serviceaccount:agent:station", UID: "sa-uid", Extra: map[string]authv1.ExtraValue{"authentication.kubernetes.io/pod-name": {"pod"}, "authentication.kubernetes.io/pod-uid": {"pod-uid"}}}}}, nil
	})
	evidence, err := Authenticate(context.Background(), client, reg, "token")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(evidence.NodePressures, []string{"DiskPressure"}) {
		t.Fatalf("pressures of the Node: %v", evidence.NodePressures)
	}
}
