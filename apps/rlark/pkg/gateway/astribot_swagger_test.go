package gateway

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

// withAstribotSwagger lays astribot/docs/swagger.yaml, the routes and fields
// this fork adds, over the official document: mappings merge key by key and
// any other value of ours replaces the official one.
func withAstribotSwagger(t *testing.T, official []byte) []byte {
	t.Helper()
	ours, err := os.ReadFile("../../../../astribot/docs/swagger.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document, overlay map[string]any
	if err := yaml.Unmarshal(official, &document); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(ours, &overlay); err != nil {
		t.Fatal(err)
	}
	mergeYAML(document, overlay)
	data, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mergeYAML(dst, src map[string]any) {
	for key, value := range src {
		into, isMap := dst[key].(map[string]any)
		from, fromMap := value.(map[string]any)
		if isMap && fromMap {
			mergeYAML(into, from)
			continue
		}
		dst[key] = value
	}
}
