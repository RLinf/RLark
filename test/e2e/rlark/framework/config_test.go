package rlarkframework

import (
	"testing"

	shared "github.com/rlinf/rlark/test/e2e/framework"
)

func TestLoadConfigDefaultsToSelfProvisionedKind(t *testing.T) {
	t.Setenv("E2E_REPO_ROOT", t.TempDir())

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != ProviderKind {
		t.Fatalf("default provider = %q, want %q", config.Provider, ProviderKind)
	}
	requirements := shared.Requirements{Clusters: []shared.ClusterRequirement{
		{Role: "scenario-cluster", ControlNodes: 1},
	}}
	if _, err := config.NewProvider(requirements); err != nil {
		t.Fatal(err)
	}
}

func TestProviderRejectsEmptyScenarioRequirements(t *testing.T) {
	t.Setenv("E2E_REPO_ROOT", t.TempDir())
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.NewProvider(shared.Requirements{}); err == nil {
		t.Fatal("provider accepted empty scenario requirements")
	}
}
