package cameracontroller

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ControllerConfig is the camera-controller configuration read from the YAML
// config file. Runtime/CLI fields (SocketPath) are not in the file — they
// live in ServerConfig and are set by the cmd from flags. This mirrors
// roscontroller.ControllerConfig so the two controllers share the same
// config-building shape: a file struct (ControllerConfig) + a runtime struct
// (ServerConfig) assembled by the cmd.
type ControllerConfig struct {
	// Cameras are camera device configurations to register at startup.
	Cameras []CameraConfig `yaml:"cameras,omitempty"`

	// HTTPAddr is the optional TCP address ("host:port" or ":port") for the
	// HTTP/JSON gateway. Empty disables HTTP. The device-plugin writes this
	// into the config file (from camera.http_addr) so it applies in both
	// local and pod mode.
	HTTPAddr string `yaml:"http_addr,omitempty"`
}

// LoadConfigFile reads and parses a camera-controller YAML config file.
// Mirrors roscontroller.LoadConfigFile.
func LoadConfigFile(path string) (ControllerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ControllerConfig{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg ControllerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ControllerConfig{}, fmt.Errorf("parse config file %s: %w", path, err)
	}

	return cfg, nil
}
