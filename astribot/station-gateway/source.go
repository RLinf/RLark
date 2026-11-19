package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// sources are the places a node's configuration can come from, in order of
// preference. Only static is required; the other two belong to enrollment.
type sources struct {
	// computed is written by the station-agent container from Station API's
	// answer to this node's enrollment.
	computed string
	// lastApplied is this node's copy of the last computed document it
	// reconciled, kept beside the private key so that a restart with Station
	// API unreachable brings the same tunnel back.
	lastApplied string
	// static is the administrator's file. It bootstraps a node that cannot
	// reach Station API without the tunnel, and it is what a node falls back
	// to before its first enrollment.
	static string
}

var passedOver string

// read returns the first document that exists and parses, and its origin.
func (s sources) read() ([]byte, *desired, string, error) {
	var firstErr error
	for _, candidate := range []struct{ origin, path string }{{"computed", s.computed}, {"last-applied", s.lastApplied}, {"static", s.static}} {
		if candidate.path == "" {
			continue
		}
		data, err := os.ReadFile(candidate.path)
		if err == nil {
			var d *desired
			if d, err = parse(data); err == nil {
				return data, d, candidate.origin, nil
			}
			// A document this program cannot read is passed over for the
			// next source, and said once: the node then runs on an older
			// document than the one it was given.
			if said := candidate.path + ": " + err.Error(); said != passedOver {
				passedOver = said
				log.Printf("the %s configuration is not usable: %v", candidate.origin, err)
			}
		}
		if firstErr == nil && !errors.Is(err, os.ErrNotExist) {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = errors.New("no configuration is available yet")
	}
	return nil, nil, "", firstErr
}

// publishIdentity lets the agent container enroll the public key. The file
// holds nothing secret.
func publishIdentity(path string, key wgtypes.Key) error {
	content, err := json.Marshal(struct {
		PublicKey string `json:"publicKey"`
	}{key.PublicKey().String()})
	if err != nil {
		return err
	}
	return replaceFile(path, bytes.NewReader(content), 0644)
}

// replaceFile puts what content reads at path in one rename. Content is
// streamed: a program copied this way is not held in memory, which the
// keeper's container limits (a machine of many cores and processes has
// little room left under them).
func replaceFile(path string, content io.Reader, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".station-gateway-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = io.Copy(f, content)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
