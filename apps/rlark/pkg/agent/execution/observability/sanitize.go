package observability

import (
	"encoding/json"
	"fmt"
)

type productionSanitizer struct{}

// ProductionSanitizer returns the mandatory production payload sanitizer.
func ProductionSanitizer() Sanitizer { return productionSanitizer{} }

func (productionSanitizer) Sanitize(_ Payload, value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode payload for sanitization: %w", err)
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, fmt.Errorf("decode payload for sanitization: %w", err)
	}
	return redact(generic, false), nil
}

func redact(value any, inEnvironmentEntry bool) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		kind, _ := typed["kind"].(string)
		for key, item := range typed {
			if sensitiveKey(key) || (kind == "Secret" && (key == "data" || key == "stringData")) || (inEnvironmentEntry && key == "value") {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = redact(item, key == "env")
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i := range typed {
			result[i] = redact(typed[i], inEnvironmentEntry)
		}
		return result
	default:
		return value
	}
}
