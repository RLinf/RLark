package addons

import (
	"bytes"
	"io"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRcloneSelectorsValidateAndDefault(t *testing.T) {
	addon, _ := Registry.Get("csi-driver-rclone")
	for _, name := range []string{"nodeSelector", "controllerNodeSelector"} {
		for _, value := range []string{"missing-equals", "a=b=c", "bad key=value", "valid/key=bad value"} {
			if _, err := addon.Render(map[string]string{name: value}, "rlark-system", "csi", "uid"); err == nil {
				t.Errorf("accepted invalid %s %q", name, value)
			}
		}
	}
	manifests, err := addon.Render(nil, "rlark-system", "csi", "uid")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range manifests {
		decoder := yaml.NewDecoder(bytes.NewReader(manifest.Raw))
		for {
			var resource map[string]any
			if err := decoder.Decode(&resource); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if resource["kind"] != "Deployment" {
				continue
			}
			pod := resource["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
			if _, ok := pod["nodeSelector"]; ok {
				t.Fatal("default controller unexpectedly restricts placement")
			}
		}
	}
}

func TestRclonePreservesExistingPodsWithOnDelete(t *testing.T) {
	addon, _ := Registry.Get("csi-driver-rclone")
	manifests, err := addon.Render(map[string]string{"nodeUpdateStrategy": "OnDelete", "nodeImagePullPolicy": "IfNotPresent"}, "rlark-system", "csi", "uid")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range manifests {
		decoder := yaml.NewDecoder(bytes.NewReader(manifest.Raw))
		for {
			var resource map[string]any
			if err := decoder.Decode(&resource); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if resource["kind"] != "DaemonSet" {
				continue
			}
			spec := resource["spec"].(map[string]any)
			strategy := spec["updateStrategy"].(map[string]any)
			if strategy["type"] != "OnDelete" || strategy["rollingUpdate"] != nil {
				t.Fatalf("unexpected strategy: %v", strategy)
			}
			pod := spec["template"].(map[string]any)["spec"].(map[string]any)
			if pod["nodeSelector"] != nil {
				t.Fatal("default node placement restricted")
			}
			for _, c := range pod["containers"].([]any) {
				if c.(map[string]any)["imagePullPolicy"] != "IfNotPresent" {
					t.Fatalf("unexpected container: %v", c)
				}
			}
		}
	}
	for _, values := range []map[string]string{{"nodeUpdateStrategy": "invalid"}, {"nodeImagePullPolicy": "Never"}} {
		if _, err := addon.Render(values, "rlark-system", "csi", "uid"); err == nil {
			t.Fatal("invalid strategy accepted")
		}
	}
}
