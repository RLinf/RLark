package distribution

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatches(t *testing.T) {
	tests := []struct {
		name      string
		selector  GlobSelector
		namespace string
		want      bool
	}{
		{"all", GlobSelector{Include: []string{"*"}}, "tenant-a", true},
		{"prefix", GlobSelector{Include: []string{"tenant-*"}}, "tenant-a", true},
		{"whole string", GlobSelector{Include: []string{"tenant-*"}}, "a-tenant-a", false},
		{"single", GlobSelector{Include: []string{"prod-?"}}, "prod-a", true},
		{"range", GlobSelector{Include: []string{"prod-[a-c]"}}, "prod-b", true},
		{"escaped", GlobSelector{Include: []string{`tenant-\*`}}, "tenant-*", true},
		{"excluded", GlobSelector{Include: []string{"tenant-*"}, Exclude: []string{"tenant-system"}}, "tenant-system", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Matches(tt.selector, tt.namespace))
		})
	}
}

func TestParse(t *testing.T) {
	definition, err := Parse(validData())
	require.NoError(t, err)
	require.Len(t, definition.Objects, 1)
	assert.Equal(t, "ConfigMap", definition.Objects[0].GetKind())
	assert.Equal(t, "runtime-config", definition.Objects[0].GetName())
	assert.Equal(t, "ServerSideApply", definition.Config.Apply.Mode)
	assert.Equal(t, "Fail", definition.Config.Apply.ConflictPolicy)
	assert.Equal(t, "Delete", definition.Config.Apply.DeletionPolicy)
}

func TestParseRejectsInvalidDeclarations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string][]byte)
		match  string
	}{
		{"empty include", func(data map[string][]byte) { data["config"] = []byte("targets:\n  namespaces:\n    include: []") }, "include"},
		{"invalid glob", func(data map[string][]byte) { data["config"] = []byte("targets:\n  namespaces:\n    include: ['[']") }, "invalid namespace pattern"},
		{"missing api version", func(data map[string][]byte) {
			data["manifests"] = []byte("- kind: ConfigMap\n  metadata:\n    name: invalid")
		}, "apiVersion"},
		{"resource version", func(data map[string][]byte) {
			data["manifests"] = []byte("- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: invalid\n    resourceVersion: '1'")
		}, "resourceVersion"},
		{"owner reference", func(data map[string][]byte) {
			data["manifests"] = []byte("- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: invalid\n    ownerReferences: []")
		}, "ownerReferences"},
		{"status", func(data map[string][]byte) {
			data["manifests"] = []byte("- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: invalid\n  status: {}")
		}, "status is forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := validData()
			tt.mutate(data)
			_, err := Parse(data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.match)
		})
	}
}

func TestParseBundle(t *testing.T) {
	definition, err := Parse(map[string][]byte{
		"config": []byte("security:\n  allowClusterScoped: true"),
		"manifests": []byte(`
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: runtime-config
    namespace: tenant-a
  data:
    mode: production
- apiVersion: v1
  kind: Namespace
  metadata:
    name: shared
`),
	})
	require.NoError(t, err)
	require.Len(t, definition.Objects, 2)
	assert.Equal(t, "ConfigMap", definition.Objects[0].GetKind())
	assert.Equal(t, "tenant-a", definition.Objects[0].GetNamespace())
	assert.Equal(t, "Namespace", definition.Objects[1].GetKind())
}

func TestParseRejectsInvalidBundle(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
		want string
	}{
		{"missing manifests", map[string][]byte{"config": []byte("{}"), "manifest": []byte("metadata:\n  name: one")}, "data.manifests is required"},
		{"empty bundle", map[string][]byte{"config": []byte("{}"), "manifests": []byte("[]")}, "at least one"},
		{"missing api version", map[string][]byte{"config": []byte("{}"), "manifests": []byte("- kind: ConfigMap\n  metadata:\n    name: one")}, "apiVersion"},
		{"runtime metadata", map[string][]byte{"config": []byte("{}"), "manifests": []byte("- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: one\n    uid: old")}, "uid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.data)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func validData() map[string][]byte {
	return map[string][]byte{
		"config": []byte(`targets:
  namespaces:
    include: ["tenant-*"]
apply:
  mode: ServerSideApply
`),
		"manifests": []byte(`- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: runtime-config
  data:
    mode: production
`),
	}
}
