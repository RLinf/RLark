package smoke

import (
	"testing"

	shared "github.com/rlinf/rlark/test/e2e/framework"
)

func TestLoadInputRequiresImmutableScenarioImages(t *testing.T) {
	t.Setenv("RLARK_SMOKE_IMAGE", "rlark:latest")
	t.Setenv("RLARK_SMOKE_UI_IMAGE", "rlark-ui:test")
	if _, err := LoadInput(shared.Config{RepoRoot: t.TempDir()}); err == nil {
		t.Fatal("mutable latest image was accepted")
	}
}

func TestLoadInputAcceptsDigests(t *testing.T) {
	t.Setenv("RLARK_SMOKE_IMAGE", "example/rlark@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("RLARK_SMOKE_UI_IMAGE", "example/ui@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if _, err := LoadInput(shared.Config{RepoRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
}
