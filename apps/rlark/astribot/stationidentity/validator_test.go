package stationidentity

import (
	"context"
	"testing"
)

type fakeReader struct {
	evidence Evidence
	err      error
}

func (f *fakeReader) Authenticate(context.Context, string) (Evidence, error) {
	return f.evidence, f.err
}

func validEvidence() Evidence {
	return Evidence{ClusterID: "c", RegisteredClusterID: "c", ServiceAccountNamespace: "agent", RegisteredNamespace: "agent", ServiceAccountName: "station", RegisteredServiceAccount: "station", TokenServiceAccountUID: "sa", RegisteredServiceAccountUID: "sa", PodServiceAccountName: "station", TokenPodName: "pod", PodName: "pod", PodNodeName: "node", NodeName: "node", TokenPodUID: "p", PodUID: "p", NodeUID: "n", TokenAuthenticated: true, TokenAudiences: []string{"station-api", "other"}, RequiredAudiences: []string{"station-api"}}
}

func TestValidateRejectsCrossClusterAndStaleEvidence(t *testing.T) {
	cases := []func(*Evidence){func(e *Evidence) { e.TokenAuthenticated = false }, func(e *Evidence) { e.ClusterID = "other" }, func(e *Evidence) { e.TokenPodUID = "old" }, func(e *Evidence) { e.NodeDeleted = true }, func(e *Evidence) { e.TokenAudiences = []string{"wrong"} }, func(e *Evidence) { e.RequiredAudiences = []string{""}; e.TokenAudiences = []string{""} }, func(e *Evidence) { e.NodeUID = "" }, func(e *Evidence) { e.TokenServiceAccountUID = "other" }, func(e *Evidence) { e.PodServiceAccountName = "other" }, func(e *Evidence) { e.TokenPodName = "other" }, func(e *Evidence) { e.PodNodeName = "other" }}
	for _, mutate := range cases {
		e := validEvidence()
		mutate(&e)
		if err := Validate(e); err == nil {
			t.Fatal("accepted invalid evidence")
		}
	}
	if err := Validate(validEvidence()); err != nil {
		t.Fatal(err)
	}
}
