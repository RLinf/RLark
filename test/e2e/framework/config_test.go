package framework

import (
	"testing"
	"time"
)

func TestRunOwnsOnlyItsLabels(t *testing.T) {
	run := &Run{ID: "run-a", RunIDLabel: "example.test/run-id"}
	if !run.ProvesOwnership(map[string]string{"example.test/run-id": "run-a"}) {
		t.Fatal("run did not recognize its own label")
	}
	if run.ProvesOwnership(map[string]string{"example.test/run-id": "run-b"}) {
		t.Fatal("run accepted a foreign label")
	}
}

func TestDurationEnvRejectsInvalidValue(t *testing.T) {
	t.Setenv("E2E_TIMEOUT", "not-a-duration")
	if _, err := DurationEnv("E2E_TIMEOUT", time.Minute); err == nil {
		t.Fatal("invalid timeout was accepted")
	}
}

func TestLoadConfigContainsOnlySharedSettings(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("E2E_REPO_ROOT", repoRoot)
	t.Setenv("E2E_ARTIFACTS", repoRoot+"/results")

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.RepoRoot != repoRoot || config.ArtifactsDir != repoRoot+"/results" {
		t.Fatalf("unexpected shared config: %#v", config)
	}
}
