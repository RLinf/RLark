package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestParseFormat verifies the string-to-Format mapping.
func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"json", FormatJSON},
		{"yaml", FormatYAML},
		{"table", FormatTable},
		{"", FormatTable},
		{"unknown", FormatTable},
		{"JSON", FormatTable}, // case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseFormat(tt.input); got != tt.want {
				t.Errorf("ParseFormat(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFormatString verifies the Format-to-string conversion.
func TestFormatString(t *testing.T) {
	tests := []struct {
		f    Format
		want string
	}{
		{FormatTable, "table"},
		{FormatJSON, "json"},
		{FormatYAML, "yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.f.String(); got != tt.want {
				t.Errorf("Format(%d).String() = %q, want %q", tt.f, got, tt.want)
			}
		})
	}
}

// TestFormatRoundTrip verifies that String -> ParseFormat is identity for the
// three canonical formats.
func TestFormatRoundTrip(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatJSON, FormatYAML} {
		if got := ParseFormat(f.String()); got != f {
			t.Errorf("round-trip %d -> %q -> %d failed", f, f.String(), got)
		}
	}
}

// TestMarshalJSON verifies that marshalJSON produces valid JSON for ordinary Go
// types (maps, structs).
func TestMarshalJSON(t *testing.T) {
	data := map[string]any{"name": "robot-0", "count": float64(3)}
	b := marshalJSON(data)
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, b)
	}
	if decoded["name"] != "robot-0" || decoded["count"] != float64(3) {
		t.Errorf("decoded = %v", decoded)
	}
	if !bytes.Contains(b, []byte("  ")) {
		t.Error("expected indented JSON output")
	}
}

// TestMarshalJSON_Struct verifies marshaling of a struct with JSON tags.
func TestMarshalJSON_Struct(t *testing.T) {
	type item struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	b := marshalJSON(item{ID: "r0", Name: "franka"})
	var got item
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "r0" || got.Name != "franka" {
		t.Errorf("got %+v", got)
	}
}

// TestMarshalYAML verifies that marshalYAML produces valid YAML for ordinary
// Go types.
func TestMarshalYAML(t *testing.T) {
	data := map[string]string{"host": "10.0.0.1", "port": "11311"}
	b := marshalYAML(data)
	var decoded map[string]string
	if err := yaml.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("invalid YAML output: %v\n%s", err, b)
	}
	if decoded["host"] != "10.0.0.1" || decoded["port"] != "11311" {
		t.Errorf("decoded = %v", decoded)
	}
}

// TestMarshalYAML_Struct verifies marshaling of a struct to YAML.
func TestMarshalYAML_Struct(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	}
	b := marshalYAML(cfg{Name: "ros", Port: 8080})
	var got cfg
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Name != "ros" || got.Port != 8080 {
		t.Errorf("got %+v", got)
	}
}

// TestPrint_Table verifies that Print calls the tableFn for FormatTable.
func TestPrint_Table(t *testing.T) {
	called := false
	capture := captureStdout(t, func() {
		Print(FormatTable, nil, func() {
			called = true
			_, _ = os.Stdout.WriteString("TABLE OUTPUT\n")
		})
	})
	if !called {
		t.Error("tableFn not called for FormatTable")
	}
	if !strings.Contains(capture, "TABLE OUTPUT") {
		t.Errorf("stdout = %q, want table output", capture)
	}
}

// TestPrint_JSON verifies that Print outputs JSON for FormatJSON.
func TestPrint_JSON(t *testing.T) {
	out := captureStdout(t, func() {
		Print(FormatJSON, map[string]string{"k": "v"}, nil)
	})
	var decoded map[string]string
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if decoded["k"] != "v" {
		t.Errorf("decoded = %v", decoded)
	}
}

// TestPrint_YAML verifies that Print outputs YAML for FormatYAML.
func TestPrint_YAML(t *testing.T) {
	out := captureStdout(t, func() {
		Print(FormatYAML, map[string]string{"k": "v"}, nil)
	})
	var decoded map[string]string
	if err := yaml.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid YAML: %v\n%s", err, out)
	}
	if decoded["k"] != "v" {
		t.Errorf("decoded = %v", decoded)
	}
}

// captureStdout runs fn while redirecting os.Stdout to a buffer, returning the
// captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}
