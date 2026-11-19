package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config contains execution settings shared by every E2E product suite.
// Product deployment inputs belong to the scenario that consumes them.
type Config struct {
	RepoRoot          string
	ArtifactsDir      string
	DefaultTimeout    time.Duration
	CleanupTimeout    time.Duration
	RetainEnvironment bool
}

func LoadConfig() (Config, error) {
	repoRoot, err := FindRepoRoot(os.Getenv("E2E_REPO_ROOT"))
	if err != nil {
		return Config{}, err
	}
	defaultTimeout, err := DurationEnv("E2E_TIMEOUT", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cleanupTimeout, err := DurationEnv("E2E_CLEANUP_TIMEOUT", 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	return Config{
		RepoRoot:          repoRoot,
		ArtifactsDir:      EnvOrDefault("E2E_ARTIFACTS", filepath.Join(repoRoot, "_artifacts", "e2e")),
		DefaultTimeout:    defaultTimeout,
		CleanupTimeout:    cleanupTimeout,
		RetainEnvironment: BoolEnv("E2E_RETAIN_ENVIRONMENT"),
	}, nil
}

func FindRepoRoot(configured string) (string, error) {
	if configured != "" {
		return filepath.Abs(configured)
	}
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.work")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("repository root containing go.work not found")
		}
		current = parent
	}
}

func EnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func DurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s=%q: %w", key, value, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return duration, nil
}

func BoolEnv(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
