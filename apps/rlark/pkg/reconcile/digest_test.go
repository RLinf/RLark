package reconcile

import (
	"strings"
	"testing"
)

func TestDigestJSONIsOrderIndependentForMapKeys(t *testing.T) {
	left, err := DigestJSON(map[string]any{"b": 2, "a": []string{"one", "two"}})
	if err != nil {
		t.Fatalf("digest left: %v", err)
	}
	right, err := DigestJSON(map[string]any{"a": []string{"one", "two"}, "b": 2})
	if err != nil {
		t.Fatalf("digest right: %v", err)
	}
	if left != right {
		t.Fatalf("expected map key order to be irrelevant, got %q and %q", left, right)
	}
	if err := left.Validate(); err != nil {
		t.Fatalf("expected canonical digest, got %v", err)
	}
}

func TestDigestJSONDistinguishesDifferentProjections(t *testing.T) {
	left, err := DigestJSON(map[string]string{"a": "one"})
	if err != nil {
		t.Fatalf("digest left: %v", err)
	}
	right, err := DigestJSON(map[string]string{"a": "two"})
	if err != nil {
		t.Fatalf("digest right: %v", err)
	}
	if left == right {
		t.Fatal("expected different projections to produce different digests")
	}
}

func TestDigestJSONRejectsUnmarshalableProjection(t *testing.T) {
	if _, err := DigestJSON(func() {}); err == nil {
		t.Fatal("expected an error for a value encoding/json cannot represent")
	}
}

func TestDigestValidate(t *testing.T) {
	canonical := string(NewDigest([]byte("payload")))
	cases := []struct {
		name   string
		digest Digest
		valid  bool
	}{
		{name: "canonical", digest: Digest(canonical), valid: true},
		{name: "empty", digest: "", valid: false},
		{name: "missing prefix", digest: Digest(strings.TrimPrefix(canonical, "sha256:")), valid: false},
		{name: "wrong algorithm", digest: Digest("md5:" + strings.TrimPrefix(canonical, "sha256:")), valid: false},
		{name: "upper case hex", digest: Digest(strings.ToUpper(canonical)), valid: false},
		{name: "truncated", digest: Digest(canonical[:len(canonical)-1]), valid: false},
		{name: "non hexadecimal", digest: Digest("sha256:" + strings.Repeat("z", 64)), valid: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.digest.Validate()
			if tc.valid && err != nil {
				t.Fatalf("expected %q to be valid, got %v", tc.digest, err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("expected %q to be rejected", tc.digest)
			}
		})
	}
}
