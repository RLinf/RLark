package deviceplugin

import (
	"log"

	"gopkg.in/yaml.v3"
)

// generateROS2Config builds the ros2-controller YAML config from the inlined
// ControllerConfig fields of ROS2Config. Returns an empty valid config (no
// robots) when no fields are set.
func generateROS2Config(cfg ROS2Config) []byte {
	data, err := yaml.Marshal(&cfg.ControllerConfig)
	if err != nil {
		log.Printf("[device-plugin] WARNING: marshal ros2-controller config: %v", err)
		return nil
	}
	return data
}
