package common

import "testing"

func TestNormalizeRegistry(t *testing.T) {
	tests := map[string]string{
		"harbor.example.com":             "harbor.example.com",
		"https://harbor.example.com/":    "harbor.example.com",
		"http://harbor.example.com:5000": "harbor.example.com:5000",
		"harbor.example.com/v2/":         "harbor.example.com",
	}
	for input, want := range tests {
		if got := NormalizeRegistry(input); got != want {
			t.Errorf("NormalizeRegistry(%q) = %q, want %q", input, got, want)
		}
	}
}
