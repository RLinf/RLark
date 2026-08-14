package deviceplugin

import (
	"log"

	"gopkg.in/yaml.v3"
)

// generateROSConfig builds the ros-controller YAML config from the inlined
// ControllerConfig fields of ROSConfig. Returns an empty valid config (no
// robots) when no fields are set.
func generateROSConfig(cfg ROSConfig) []byte {
	data, err := yaml.Marshal(&cfg.ControllerConfig)
	if err != nil {
		log.Printf("[device-plugin] WARNING: marshal ros-controller config: %v", err)
		return nil
	}
	return data
}
