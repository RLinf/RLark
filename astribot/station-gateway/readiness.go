package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const readinessPath = "/tmp/rlark-station-gateway-ready.json"
const readinessTTL = 30 * time.Second

type readiness struct {
	ConfigHash [32]byte  `json:"configHash"`
	CheckedAt  time.Time `json:"checkedAt"`
}

// writeReady publishes only a completed successful reconciliation. The file is
// container-local and is replaced atomically so a concurrent probe cannot see
// a partial update.
func writeReady(path string, data []byte, now time.Time) error {
	content, err := json.Marshal(readiness{ConfigHash: sha256.Sum256(data), CheckedAt: now.UTC()})
	if err != nil {
		return err
	}
	return replaceFile(path, bytes.NewReader(content), 0600)
}

// checkReady checks reconciliation freshness, not peer connectivity.
func checkReady(data []byte, path string, now time.Time) error {
	state, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read readiness: %w", err)
	}
	var r readiness
	if err := json.Unmarshal(state, &r); err != nil {
		return fmt.Errorf("decode readiness: %w", err)
	}
	age := now.Sub(r.CheckedAt)
	if r.CheckedAt.IsZero() || age < 0 || age > readinessTTL {
		return errors.New("reconciliation is stale")
	}
	if r.ConfigHash != sha256.Sum256(data) {
		return errors.New("configuration has changed since reconciliation")
	}
	return nil
}
