//go:build rlark_execution_trace_unsafe

package observability

// UnsafeSanitizer is available only in explicit diagnostic builds. Standard
// binaries and release images must never enable rlark_execution_trace_unsafe.
func UnsafeSanitizer() Sanitizer { return unsafeSanitizer{} }

type unsafeSanitizer struct{}

func (unsafeSanitizer) Sanitize(_ Payload, value any) (any, error) { return value, nil }
