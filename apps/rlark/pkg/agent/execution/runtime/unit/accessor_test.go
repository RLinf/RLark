package unit

import (
	"encoding/json"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
)

func TestReadWriteNestedOpaqueUnit(t *testing.T) {
	manifest := []byte(`{"spec":{"worker":{"template":{"value":"old"}}},"other":true}`)
	value, err := Read(manifest, "spec.worker.template")
	if err != nil || string(value) != `{"value":"old"}` {
		t.Fatalf("value=%s error=%v", value, err)
	}
	result, err := Write(manifest, "spec.worker.template", []byte(`{"value":"new"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(result, &got); err != nil || got["other"] != true {
		t.Fatalf("manifest=%s error=%v", result, err)
	}
	value, err = Read(result, "spec.worker.template")
	if err != nil || string(value) != `{"value":"new"}` {
		t.Fatalf("value=%s error=%v", value, err)
	}
}

func TestReadWriteRootUnit(t *testing.T) {
	value, err := Read([]byte(`{"value":"old"}`), ".")
	if err != nil || string(value) != `{"value":"old"}` {
		t.Fatalf("value=%s error=%v", value, err)
	}
	result, err := Write(value, ".", []byte(`{"value":"new"}`))
	if err != nil || string(result) != `{"value":"new"}` {
		t.Fatalf("result=%s error=%v", result, err)
	}
}

func TestAccessorRejectsInvalidOrMissingLocator(t *testing.T) {
	for _, locator := range []string{"", ".spec", "spec.items[0]", "spec.missing"} {
		t.Run(locator, func(t *testing.T) {
			if _, err := Read([]byte(`{"spec":{"items":[]}}`), locator); !fault.Is(err, fault.InvalidArgument) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
