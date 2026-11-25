package utils

import "encoding/json"

// JSONEqual compares two values by their canonical JSON serialization.
// Struct fields are serialized in definition order; map keys are sorted.
// Two values with the same content always produce the same serialization.
func JSONEqual(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}
