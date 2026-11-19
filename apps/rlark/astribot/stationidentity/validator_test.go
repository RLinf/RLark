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
	return Evidence{ClusterID: "c", RegisteredClusterID: "c", ServiceAccountNamespace: "agent", RegisteredNamespace: "agent", ServiceAccountName: "station", RegisteredServiceAccount: "station", TokenServiceAccountUID: "sa", RegisteredServiceAccountUID: "sa", PodServiceAccountName: "station", TokenPodName: "pod", PodName: "pod", PodNodeName: "node", NodeName: "node", TokenPodUID: "p", PodUID: "p", NodeUID: "n", RegisteredNodeUID: "n", StationUID: "s", RegisteredStationUID: "s", RegisteredGeneration: 1, TokenAuthenticated: true, TokenAudiences: []string{"station-api", "other"}, RequiredAudiences: []string{"station-api"}, Generation: 1}
}

func TestValidateRejectsCrossClusterAndStaleEvidence(t *testing.T) {
	cases := []func(*Evidence){func(e *Evidence) { e.TokenAuthenticated = false }, func(e *Evidence) { e.ClusterID = "other" }, func(e *Evidence) { e.TokenPodUID = "old" }, func(e *Evidence) { e.NodeDeleted = true }, func(e *Evidence) { e.TokenAudiences = []string{"wrong"} }, func(e *Evidence) { e.RequiredAudiences = []string{""}; e.TokenAudiences = []string{""} }, func(e *Evidence) { e.Generation = 0 }, func(e *Evidence) { e.RegisteredGeneration = 2 }, func(e *Evidence) { e.RegisteredStationUID = "other" }, func(e *Evidence) { e.BindingInactive = true }, func(e *Evidence) { e.TokenServiceAccountUID = "other" }, func(e *Evidence) { e.PodServiceAccountName = "other" }, func(e *Evidence) { e.TokenPodName = "other" }, func(e *Evidence) { e.PodNodeName = "other" }}
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

func TestValidateAllowsVerifiedNodeBeforeStationBinding(t *testing.T) {
	evidence := validEvidence()
	evidence.StationUID = ""
	evidence.RegisteredStationUID = ""
	evidence.Generation = 0
	evidence.RegisteredGeneration = 0
	if err := Validate(evidence); err != nil {
		t.Fatalf("valid unbound Node evidence rejected: %v", err)
	}
	evidence.RegisteredNodeUID = "replacement-node"
	if err := Validate(evidence); err == nil {
		t.Fatal("unbound evidence accepted a mismatched Node UID")
	}
}
