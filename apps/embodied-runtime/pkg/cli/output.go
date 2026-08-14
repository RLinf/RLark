// Package cli provides shared output formatting for CLI commands.
// Supports -o table / -o json / -o yaml output modes, with protojson
// awareness for protobuf message types.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

// Format represents a supported output format.
type Format int

// Supported output formats.
const (
	FormatTable Format = iota
	FormatJSON
	FormatYAML
)

// ParseFormat parses a format string into a Format value.
// Accepts "json", "yaml"; anything else returns FormatTable.
func ParseFormat(s string) Format {
	switch s {
	case "json":
		return FormatJSON
	case "yaml":
		return FormatYAML
	default:
		return FormatTable
	}
}

// String returns the flag value for the format.
func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatYAML:
		return "yaml"
	default:
		return "table"
	}
}

// Print prints data according to the output format.
// For table format, it calls tableFn to render the output.
// For json/yaml, it serializes the data — using protojson for
// proto.Message inputs, and standard encoding for everything else.
func Print(format Format, data any, tableFn func()) {
	switch format {
	case FormatJSON:
		b := marshalJSON(data)
		fmt.Println(string(b))

	case FormatYAML:
		b := marshalYAML(data)
		fmt.Print(string(b))

	default:
		tableFn()
	}
}

// FormatFromCmd reads the --format / -o flag from a cobra command and
// returns the corresponding Format. Defaults to FormatTable if the flag
// is missing or unrecognized.
func FormatFromCmd(cmd *cobra.Command) Format {
	f := cmd.Flag("format")
	if f == nil {
		return FormatTable
	}
	return ParseFormat(f.Value.String())
}

// AddFormatFlag registers the standard --format / -o flag on a cobra command.
func AddFormatFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("format", "o", "table", "Output format: table, json, or yaml")
}

// marshalJSON marshals data to JSON. Uses protojson for proto.Message
// inputs (canonical protobuf JSON), and encoding/json for everything else.
func marshalJSON(data any) []byte {
	if msg, ok := data.(proto.Message); ok {
		m := protojson.MarshalOptions{
			EmitUnpopulated: true,
			Indent:          "  ",
		}
		b, err := m.Marshal(msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: protojson marshal: %v\n", err)
			os.Exit(1)
		}
		return b
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: json marshal: %v\n", err)
		os.Exit(1)
	}
	return b
}

// marshalYAML marshals data to YAML. For proto.Message inputs, it first
// converts to protojson, then via map to YAML for canonical proto output.
func marshalYAML(data any) []byte {
	if msg, ok := data.(proto.Message); ok {
		m := protojson.MarshalOptions{
			EmitUnpopulated: true,
		}
		jsonBytes, err := m.Marshal(msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: protojson→yaml marshal: %v\n", err)
			os.Exit(1)
		}
		var raw any
		if err := json.Unmarshal(jsonBytes, &raw); err != nil {
			fmt.Fprintf(os.Stderr, "error: protojson→yaml decode: %v\n", err)
			os.Exit(1)
		}
		b, err := yaml.Marshal(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: yaml marshal: %v\n", err)
			os.Exit(1)
		}
		return b
	}

	b, err := yaml.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: yaml marshal: %v\n", err)
		os.Exit(1)
	}
	return b
}
