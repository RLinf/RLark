package logquery

// sensitiveFields lists, per backend, the config keys that must be redacted
// before the config is returned to clients (UI, logs, etc.).
var sensitiveFields = map[string][]string{
	"sls": {"accessKeyId", "accessKeySecret"},
}

const maskedValue = "****"

// MaskSensitiveFields returns a copy of cfg with backend-specific sensitive
// fields replaced by a placeholder. Unknown backends are returned as-is.
func MaskSensitiveFields(backend string, cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	out := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	for _, k := range sensitiveFields[backend] {
		if _, ok := out[k]; ok {
			out[k] = maskedValue
		}
	}
	return out
}
