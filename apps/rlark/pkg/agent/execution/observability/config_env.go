package observability

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	// EnvPayloads is a comma-separated list of enabled detailed payload names.
	EnvPayloads = "RLARK_EXECUTION_TRACE_PAYLOADS"
	// EnvMaxBytes sets the maximum encoded bytes retained for one event payload.
	EnvMaxBytes = "RLARK_EXECUTION_TRACE_MAX_BYTES"
)

// EnvConfigSource reads a dynamic configuration snapshot from environment variables.
type EnvConfigSource struct{}

// Snapshot parses the current environment. Safe retains the last valid snapshot
// when this method rejects a dynamic update.
func (EnvConfigSource) Snapshot() (Config, error) {
	config := Config{Payloads: map[Payload]bool{}, MaxEventBytes: DefaultMaxEventBytes}
	known := map[string]Payload{
		string(PayloadTaskSnapshot): PayloadTaskSnapshot,
		string(PayloadPlan):         PayloadPlan,
		string(PayloadOperations):   PayloadOperations,
		string(PayloadManifests):    PayloadManifests,
	}
	for _, item := range strings.Split(os.Getenv(EnvPayloads), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		payload, found := known[item]
		if !found {
			return Config{}, fmt.Errorf("unknown execution trace payload %q", item)
		}
		config.Payloads[payload] = true
	}
	if raw := strings.TrimSpace(os.Getenv(EnvMaxBytes)); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("execution trace max bytes must be positive")
		}
		config.MaxEventBytes = value
	}
	return config, nil
}
