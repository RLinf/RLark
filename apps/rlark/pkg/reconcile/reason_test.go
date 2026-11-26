package reconcile

import (
	"strings"
	"testing"
)

func TestValidateReason(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		valid  bool
	}{
		{name: "upper camel case", reason: "OwnershipUnproven", valid: true},
		{name: "with digits", reason: "Retry5xx", valid: true},
		{name: "single letter", reason: "A", valid: true},
		{name: "empty", reason: "", valid: false},
		{name: "lower case first letter", reason: "ownershipUnproven", valid: false},
		{name: "digit first", reason: "5xxRetry", valid: false},
		{name: "contains dot", reason: "Ownership.Unproven", valid: false},
		{name: "contains dash", reason: "Ownership-Unproven", valid: false},
		{name: "contains space", reason: "Ownership Unproven", valid: false},
		{name: "contains underscore", reason: "Ownership_Unproven", valid: false},
		{name: "non ascii", reason: "归属不可证明", valid: false},
		{name: "too long", reason: "A" + strings.Repeat("b", MaxReasonLength), valid: false},
		{name: "at length limit", reason: "A" + strings.Repeat("b", MaxReasonLength-1), valid: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateReason(tc.reason)
			if tc.valid && err != nil {
				t.Fatalf("expected %q to be valid, got %v", tc.reason, err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("expected %q to be rejected", tc.reason)
			}
		})
	}
}
