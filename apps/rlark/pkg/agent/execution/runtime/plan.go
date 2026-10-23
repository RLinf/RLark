package runtime

import (
	"errors"
	"fmt"
)

// ErrObjectDependencyCycle indicates a cycle in DesiredObject dependencies.
var ErrObjectDependencyCycle = errors.New("runtime object dependency cycle")

// ValidateObjects validates logical names, ownership, references, and DAG shape.
func ValidateObjects(objects []DesiredObject) error {
	byName := make(map[string]DesiredObject, len(objects))
	for _, object := range objects {
		if object.LogicalName == "" {
			return errors.New("runtime object logical name is required")
		}
		if _, exists := byName[object.LogicalName]; exists {
			return fmt.Errorf("duplicate runtime object logical name %q", object.LogicalName)
		}
		if err := object.Ownership.Validate(); err != nil {
			return fmt.Errorf("runtime object %q ownership: %w", object.LogicalName, err)
		}
		byName[object.LogicalName] = object
	}

	for _, object := range objects {
		for _, dependency := range object.DependsOn {
			if _, exists := byName[dependency]; !exists {
				return fmt.Errorf("runtime object %q depends on unknown object %q", object.LogicalName, dependency)
			}
		}
	}

	seen := make(map[string]bool, len(objects))
	active := make(map[string]bool, len(objects))
	var visit func(string) error
	visit = func(name string) error {
		if active[name] {
			return fmt.Errorf("%w at %q", ErrObjectDependencyCycle, name)
		}
		if seen[name] {
			return nil
		}
		active[name] = true
		for _, dependency := range byName[name].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		active[name] = false
		seen[name] = true
		return nil
	}

	for name := range byName {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}
