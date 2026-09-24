package logquery

import "testing"

func TestValidateConfigNone(t *testing.T) {
	if err := ValidateConfig(&Config{Backend: "none"}); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}
