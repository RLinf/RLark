package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadinessTracksReconciliationAndConfig(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "ready")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data := []byte(`{"interface":"wg-site"}`)
	config := data
	if err := checkReady(config, state, now); err == nil {
		t.Fatal("ready without successful reconciliation")
	}
	if err := writeReady(state, data, now); err != nil {
		t.Fatal(err)
	}
	if err := checkReady(config, state, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := checkReady(config, state, now.Add(readinessTTL+time.Second)); err == nil {
		t.Fatal("accepted stale reconciliation")
	}
	if err := checkReady(config, state, now.Add(-time.Second)); err == nil {
		t.Fatal("accepted future timestamp")
	}
	config = []byte("changed")
	if err := checkReady(config, state, now); err == nil {
		t.Fatal("accepted unapplied config")
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if err := checkReady(config, state, now); err == nil {
		t.Fatal("ready after failure cleared state")
	}
	if err := writeReady(state, []byte("changed"), now); err != nil {
		t.Fatal(err)
	}
	if err := checkReady(config, state, now); err != nil {
		t.Fatal("did not recover:", err)
	}
}
