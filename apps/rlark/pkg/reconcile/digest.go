package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const digestPrefix = "sha256:"

// Digest is a canonical SHA-256 content digest.
//
// A digest summarises a desired-state projection that one writer decided on. It
// is deliberately not a summary of a live object: comparing a recorded digest
// with a freshly computed one answers "is what I would write now the same as
// what I wrote last time", which is stable against defaulting, admission
// mutation and fields owned by other writers. Comparing live representations
// instead makes any foreign write look like a required change and produces
// permanent rewrite churn.
type Digest string

// NewDigest hashes a stable byte projection.
func NewDigest(value []byte) Digest {
	sum := sha256.Sum256(value)
	return Digest(digestPrefix + hex.EncodeToString(sum[:]))
}

// DigestJSON hashes the JSON representation of value. Callers must supply a
// deterministic projection: maps with string keys are ordered by encoding/json,
// but any field whose bytes vary between equivalent inputs breaks comparison.
func DigestJSON(value any) (Digest, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return NewDigest(data), nil
}

// Validate reports whether digest uses the canonical sha256:<lowercase-hex> form.
func (d Digest) Validate() error {
	value := string(d)
	if !strings.HasPrefix(value, digestPrefix) || len(value) != len(digestPrefix)+sha256.Size*2 {
		return errors.New("digest must use sha256:<lowercase-hex>")
	}
	encoded := strings.TrimPrefix(value, digestPrefix)
	if encoded != strings.ToLower(encoded) {
		return errors.New("digest must use lowercase hexadecimal")
	}
	_, err := hex.DecodeString(encoded)
	return err
}
