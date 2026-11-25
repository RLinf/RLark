package agent

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// EffectiveConfig contains startup-only typed configuration and non-secret
// diagnostics. Sources name the input layer, never secret values.
type EffectiveConfig struct {
	Config  Config
	Sources map[string]string
	Options map[string]string
}

// InputError preserves parser error chains without rendering secret input.
type InputError struct {
	Message string
	Cause   error
}

func (e *InputError) Error() string { return e.Message }
func (e *InputError) Unwrap() error { return e.Cause }

var configPathOptions = map[string]bool{"client-cert": true, "client-key": true, "ca-cert": true, "kubeconfig": true, "kubelet-dir": true, "containerd-socket": true, "nodeserver-unix-socket": true}

// LoadStartupConfig reuses the registered flag contracts for a strict v1 YAML
// options map. Explicit CLI wins over file, then compatibility environment/defaults.
// It reads configuration only: no clients, sockets, credentials or workers.
func LoadStartupConfig(path string, cli *pflag.FlagSet) (EffectiveConfig, error) {
	c := DefaultConfig()
	fs := pflag.NewFlagSet("effective-agent-config", pflag.ContinueOnError)
	c.SetupFlags(fs)
	result := EffectiveConfig{Sources: map[string]string{}, Options: map[string]string{}}
	fs.VisitAll(func(f *pflag.Flag) { result.Sources[f.Name] = "default" })
	for name, env := range map[string]string{"node-name": "NODE_NAME", "kubeconfig": "KUBECONFIG", "tensorboard-image": "RLARK_TENSORBOARD_IMAGE", "restartable-init-containers": "KUBERNETES_SUPPORT_SIDECAR_FEATURE"} {
		if os.Getenv(env) != "" {
			result.Sources[name] = "environment:" + env
		}
	}
	if os.Getenv("HOSTNAME") != "" {
		result.Sources["leader-election-id"] = "environment:HOSTNAME"
	} else if os.Getenv("POD_NAME") != "" {
		result.Sources["leader-election-id"] = "environment:POD_NAME"
	}
	if path != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return EffectiveConfig{}, fmt.Errorf("resolve config path: %w", err)
		}
		data, err := os.ReadFile(absolute)
		if err != nil {
			return EffectiveConfig{}, &InputError{Message: "cannot read agent config file", Cause: err}
		}
		var syntax yaml.Node
		if err := yaml.Unmarshal(data, &syntax); err != nil {
			return EffectiveConfig{}, &InputError{Message: "invalid agent config YAML", Cause: err}
		}
		if !validStartupSyntax(&syntax) {
			return EffectiveConfig{}, fmt.Errorf("agent config requires a mapping with an options map and no aliases, merge keys or nulls")
		}
		var document struct {
			Version string               `yaml:"version"`
			Options map[string]yaml.Node `yaml:"options"`
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&document); err != nil {
			return EffectiveConfig{}, &InputError{Message: "invalid agent config YAML (unknown/duplicate field or malformed input)", Cause: err}
		}
		var extra yaml.Node
		if err := decoder.Decode(&extra); err != io.EOF {
			return EffectiveConfig{}, &InputError{Message: "agent config must contain one YAML document", Cause: err}
		}
		if document.Version != "v1" {
			return EffectiveConfig{}, fmt.Errorf("agent config version must be v1")
		}
		keys := make([]string, 0, len(document.Options))
		for key := range document.Options {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, name := range keys {
			flag := fs.Lookup(name)
			if flag == nil {
				return EffectiveConfig{}, fmt.Errorf("unknown agent config option %q", name)
			}
			node := document.Options[name]
			if node.Kind != yaml.ScalarNode || node.Tag == "!!null" || node.Tag == "!!alias" {
				return EffectiveConfig{}, fmt.Errorf("option %q must be a non-null scalar", name)
			}
			kind := flag.Value.Type()
			if !validOptionScalar(kind, node.Tag) {
				return EffectiveConfig{}, fmt.Errorf("option %q has an invalid YAML type", name)
			}
			value := node.Value
			if configPathOptions[name] && value != "" && !filepath.IsAbs(value) {
				value = filepath.Join(filepath.Dir(absolute), value)
			}
			if err := fs.Set(name, value); err != nil {
				return EffectiveConfig{}, &InputError{Message: fmt.Sprintf("invalid agent config option %q", name), Cause: err}
			}
			result.Sources[name] = "file"
		}
	}
	if cli != nil {
		var inputErr error
		cli.Visit(func(flag *pflag.Flag) {
			if fs.Lookup(flag.Name) == nil || inputErr != nil {
				return
			}
			if err := fs.Set(flag.Name, flag.Value.String()); err != nil {
				inputErr = &InputError{Message: fmt.Sprintf("invalid CLI option %q", flag.Name), Cause: err}
				return
			}
			result.Sources[flag.Name] = "cli"
		})
		if inputErr != nil {
			return EffectiveConfig{}, inputErr
		}
	}
	c.AgentType = strings.ToLower(c.AgentType)
	c.Execution.Image = c.Image
	// Resolve CLI/environment paths against startup cwd; file paths were resolved
	// against the config directory before overlay. No existence check is performed.
	for name := range configPathOptions {
		flag := fs.Lookup(name)
		if flag.Value.String() == "" {
			continue
		}
		absolute, err := filepath.Abs(flag.Value.String())
		if err != nil {
			return EffectiveConfig{}, fmt.Errorf("resolve option %q path: %w", name, err)
		}
		if err := fs.Set(name, absolute); err != nil {
			return EffectiveConfig{}, fmt.Errorf("resolve option %q", name)
		}
	}
	for _, name := range []string{"node-startup", "embedded-node-metrics-bind-address", "tensorboard-image", "restartable-init-containers"} {
		if result.Sources[name] == "file" || result.Sources[name] == "cli" {
			if c.Mode != "cluster" {
				return EffectiveConfig{}, fmt.Errorf("option %q is cluster-only", name)
			}
			if name == "embedded-node-metrics-bind-address" && c.NodeStartup != "embedded" {
				return EffectiveConfig{}, fmt.Errorf("option %q requires embedded node startup", name)
			}
		}
	}
	if err := c.Validate(); err != nil {
		return EffectiveConfig{}, err
	}
	fs.VisitAll(func(f *pflag.Flag) {
		value := f.Value.String()
		// Diagnostics reveal sources but never arbitrary string input, including
		// URLs, identity strings, paths and key material.
		if f.Value.Type() == "string" {
			value = "<redacted>"
			if valueString := f.Value.String(); valueString == "" {
				value = ""
			}
		}
		result.Options[f.Name] = value
	})
	result.Options["mode"], result.Options["agent-type"], result.Options["node-startup"] = c.Mode, c.AgentType, c.NodeStartup
	result.Config = c
	return result, nil
}

func validOptionScalar(kind, tag string) bool {
	switch kind {
	case "bool":
		return tag == "!!bool"
	case "int", "int64":
		return tag == "!!int"
	case "float32":
		return tag == "!!float" || tag == "!!int"
	case "string", "duration":
		return tag == "!!str"
	default:
		return false
	}
}

func validStartupSyntax(document *yaml.Node) bool {
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return false
	}
	root := document.Content[0]
	hasOptions := false
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "options" {
			hasOptions = root.Content[i+1].Kind == yaml.MappingNode
		}
	}
	var valid func(*yaml.Node) bool
	valid = func(node *yaml.Node) bool {
		if node.Kind == yaml.AliasNode || node.Tag == "!!merge" || node.Tag == "!!null" {
			return false
		}
		for _, child := range node.Content {
			if !valid(child) {
				return false
			}
		}
		return true
	}
	return hasOptions && valid(document)
}
