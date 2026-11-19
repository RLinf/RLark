package framework

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var invalidNameCharacter = regexp.MustCompile(`[^a-z0-9-]+`)

type Run struct {
	ID           string
	Spec         string
	NamePrefix   string
	RunIDLabel   string
	SpecLabel    string
	Labels       map[string]string
	ArtifactsDir string
}

type RunConfig struct {
	Framework   Config
	LabelPrefix string
}

func NewRun(config RunConfig, spec string) (*Run, error) {
	spec = sanitizeName(spec)
	if spec == "" {
		return nil, fmt.Errorf("spec name must not be empty")
	}
	labelPrefix := strings.TrimSuffix(strings.TrimSpace(config.LabelPrefix), "/")
	if labelPrefix == "" {
		return nil, fmt.Errorf("label prefix must not be empty")
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("generate run ID: %w", err)
	}
	id := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102t150405z"), hex.EncodeToString(random))
	run := &Run{
		ID:           id,
		Spec:         spec,
		NamePrefix:   "e2e-" + id + "-" + spec,
		RunIDLabel:   labelPrefix + "/run-id",
		SpecLabel:    labelPrefix + "/spec",
		ArtifactsDir: filepath.Join(config.Framework.ArtifactsDir, id, spec),
	}
	run.Labels = map[string]string{
		run.RunIDLabel: id,
		run.SpecLabel:  spec,
	}
	if err := os.MkdirAll(run.ArtifactsDir, 0o750); err != nil {
		return nil, fmt.Errorf("create artifacts directory: %w", err)
	}
	return run, nil
}

func (r *Run) Name(suffix string) string {
	name := r.NamePrefix
	if clean := sanitizeName(suffix); clean != "" {
		name += "-" + clean
	}
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = invalidNameCharacter.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
