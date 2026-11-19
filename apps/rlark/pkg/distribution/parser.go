package distribution

import (
	"fmt"
	"path"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	maxPayloadSize = 512 * 1024
	maxBundleItems = 100
)

func Parse(data map[string][]byte) (Definition, error) {
	if len(data["config"]) == 0 {
		return Definition{}, fmt.Errorf("data.config is required")
	}
	if len(data["manifests"]) == 0 {
		return Definition{}, fmt.Errorf("data.manifests is required")
	}
	if len(data["manifests"]) > maxPayloadSize {
		return Definition{}, fmt.Errorf("manifests exceeds %d bytes", maxPayloadSize)
	}

	var config Config
	if err := yaml.Unmarshal(data["config"], &config); err != nil {
		return Definition{}, fmt.Errorf("parse config: %w", err)
	}
	if config.Apply.Mode == "" {
		config.Apply.Mode = "ServerSideApply"
	}
	if config.Apply.ConflictPolicy == "" {
		config.Apply.ConflictPolicy = "Fail"
	}
	if config.Apply.DeletionPolicy == "" {
		config.Apply.DeletionPolicy = "Delete"
	}
	if config.Apply.Mode != "ServerSideApply" {
		return Definition{}, fmt.Errorf("unsupported apply mode %q", config.Apply.Mode)
	}
	if config.Apply.ConflictPolicy != "Fail" && config.Apply.ConflictPolicy != "Force" {
		return Definition{}, fmt.Errorf("unsupported conflict policy %q", config.Apply.ConflictPolicy)
	}
	if config.Apply.DeletionPolicy != "Delete" && config.Apply.DeletionPolicy != "Orphan" {
		return Definition{}, fmt.Errorf("unsupported deletion policy %q", config.Apply.DeletionPolicy)
	}
	if config.Targets.Namespaces != nil {
		if err := validateSelector(*config.Targets.Namespaces); err != nil {
			return Definition{}, err
		}
	}

	var raw []map[string]any
	if err := yaml.Unmarshal(data["manifests"], &raw); err != nil {
		return Definition{}, fmt.Errorf("parse manifests: %w", err)
	}
	if len(raw) == 0 {
		return Definition{}, fmt.Errorf("manifests must contain at least one object")
	}
	if len(raw) > maxBundleItems {
		return Definition{}, fmt.Errorf("manifests contains %d objects, limit is %d", len(raw), maxBundleItems)
	}
	objects := make([]*unstructured.Unstructured, 0, len(raw))
	for index, content := range raw {
		object := &unstructured.Unstructured{Object: content}
		if err := validateObject(object); err != nil {
			return Definition{}, fmt.Errorf("manifest %d: %w", index, err)
		}
		objects = append(objects, object)
	}
	return Definition{Config: config, Objects: objects}, nil
}

func validateObject(object *unstructured.Unstructured) error {
	if object.GetAPIVersion() == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if object.GetKind() == "" {
		return fmt.Errorf("kind is required")
	}
	if object.GetName() == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if object.GetGenerateName() != "" {
		return fmt.Errorf("metadata.generateName is forbidden")
	}
	metadata, _, _ := unstructured.NestedMap(object.Object, "metadata")
	for _, key := range []string{"uid", "resourceVersion", "managedFields", "ownerReferences", "finalizers", "deletionTimestamp"} {
		if _, ok := metadata[key]; ok {
			return fmt.Errorf("metadata.%s is forbidden", key)
		}
	}
	if _, ok := object.Object["status"]; ok {
		return fmt.Errorf("status is forbidden")
	}
	return nil
}

func validateSelector(selector GlobSelector) error {
	if len(selector.Include) == 0 {
		return fmt.Errorf("namespace include must not be empty")
	}
	for _, pattern := range append(append([]string{}, selector.Include...), selector.Exclude...) {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("invalid namespace pattern %q: %w", pattern, err)
		}
	}
	return nil
}

func Matches(selector GlobSelector, name string) bool {
	include := false
	for _, pattern := range selector.Include {
		matched, _ := path.Match(pattern, name)
		if matched {
			include = true
			break
		}
	}
	if !include {
		return false
	}
	for _, pattern := range selector.Exclude {
		matched, _ := path.Match(pattern, name)
		if matched {
			return false
		}
	}
	return true
}
