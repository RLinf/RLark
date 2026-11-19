package plan

import "testing"

func TestRequirementsEmpty(t *testing.T) {
	tests := map[string]Requirements{
		"empty": {},
		"environment": {
			Environments: []EnvironmentRequirement{{}},
		},
	}
	for name, requirements := range tests {
		t.Run(name, func(t *testing.T) {
			if got, want := requirements.Empty(), name == "empty"; got != want {
				t.Fatalf("Empty()=%t, want %t", got, want)
			}
		})
	}
}
