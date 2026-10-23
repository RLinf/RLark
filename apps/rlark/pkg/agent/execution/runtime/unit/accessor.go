// Package unit provides Runtime-neutral access to declared execution-unit
// values inside typed manifests. It treats unit values as opaque JSON and does
// not interpret Runtime, workload, or unit semantics.
package unit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
)

// Read returns a copy of the opaque JSON value at a closed static locator.
func Read(manifest []byte, locator string) ([]byte, error) {
	path, err := StaticPath(locator)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(manifest, &root); err != nil {
		return nil, fault.Wrap(fault.InvalidArgument, "decode unit manifest", err)
	}
	value, err := nestedValue(root, path)
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(value)
	if err != nil {
		return nil, fault.Wrap(fault.Internal, "encode unit value", err)
	}
	return result, nil
}

// Write returns a new manifest with opaque JSON value written at a closed
// static locator. The locator must already exist; Write never invents a path.
func Write(manifest []byte, locator string, value []byte) ([]byte, error) {
	path, err := StaticPath(locator)
	if err != nil {
		return nil, err
	}
	var root, replacement any
	if err := json.Unmarshal(manifest, &root); err != nil {
		return nil, fault.Wrap(fault.InvalidArgument, "decode unit manifest", err)
	}
	if err := json.Unmarshal(value, &replacement); err != nil {
		return nil, fault.Wrap(fault.InvalidArgument, "decode unit value", err)
	}
	if len(path) == 0 {
		root = replacement
	} else {
		if _, err := nestedValue(root, path); err != nil {
			return nil, err
		}
		if err := setNestedValue(root, path, replacement); err != nil {
			return nil, err
		}
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, fault.Wrap(fault.Internal, "encode unit manifest", err)
	}
	return result, nil
}

// StaticPath validates a dot-separated field locator. A single dot denotes the
// manifest root. Array indexing, JSONPath operators, and empty fields are not
// accepted.
func StaticPath(locator string) ([]string, error) {
	if locator == "." {
		return nil, nil
	}
	if locator == "" || strings.HasPrefix(locator, ".") || strings.HasSuffix(locator, ".") {
		return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("invalid unit locator %q", locator))
	}
	parts := strings.Split(locator, ".")
	for _, part := range parts {
		if part == "" || strings.ContainsAny(part, "[]$*\\/") {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("invalid unit locator %q", locator))
		}
	}
	return parts, nil
}

func nestedValue(root any, path []string) (any, error) {
	current := root
	for _, field := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("unit locator crosses non-object field %q", field))
		}
		value, found := object[field]
		if !found {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("unit locator field %q is missing", field))
		}
		current = value
	}
	return current, nil
}

func setNestedValue(root any, path []string, value any) error {
	current := root
	for _, field := range path[:len(path)-1] {
		object, ok := current.(map[string]any)
		if !ok {
			return fault.New(fault.InvalidArgument, fmt.Sprintf("unit locator crosses non-object field %q", field))
		}
		current = object[field]
	}
	object, ok := current.(map[string]any)
	if !ok {
		return fault.New(fault.InvalidArgument, "unit locator parent is not an object")
	}
	object[path[len(path)-1]] = value
	return nil
}
