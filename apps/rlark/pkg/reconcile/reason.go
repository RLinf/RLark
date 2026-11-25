package reconcile

import "fmt"

// MaxReasonLength bounds the length of a reason so it stays usable as a metric
// label value and as a Kubernetes Event reason.
const MaxReasonLength = 63

// ValidateReason reports whether s is usable as a Result.Reason,
// StatusIntent.ReasonCode, or Condition reason.
//
// A reason is data, not a message. It is consumed as a metric label value, an
// Event reason and a Condition reason, all of which require a bounded set of
// values: a reason built from an object name, a node name or an error string
// turns one metric series into unboundedly many and makes conditions
// unmatchable by automation. This function enforces the shape that keeps the
// set bounded, namely a non-empty UpperCamelCase identifier of ASCII letters
// and digits only.
//
// Shape enforcement cannot prove low cardinality on its own, because a
// dynamically built string can still look like an identifier. The companion
// lint test rejects reasons that are not constant expressions; this function
// covers the constants themselves.
func ValidateReason(s string) error {
	if s == "" {
		return fmt.Errorf("reason must not be empty")
	}
	if len(s) > MaxReasonLength {
		return fmt.Errorf("reason %q is longer than %d characters", s, MaxReasonLength)
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		default:
			return fmt.Errorf("reason %q must contain ASCII letters and digits only, found %q", s, r)
		}
	}
	if first := rune(s[0]); first < 'A' || first > 'Z' {
		return fmt.Errorf("reason %q must start with an upper-case letter", s)
	}
	return nil
}
