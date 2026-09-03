package addons

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRcloneControllerIsPrivilegedForBidirectionalMounts(t *testing.T) {
	addon, ok := Registry.Get("csi-driver-rclone")
	if !ok {
		t.Fatal("csi-driver-rclone addon is not registered")
	}

	manifests, err := addon.Render(nil, "rlark-system", "csi-driver-rclone", "test-uid")
	if err != nil {
		t.Fatalf("render addon: %v", err)
	}

	for _, manifest := range manifests {
		var resource map[string]any
		if err := yaml.Unmarshal(manifest.Raw, &resource); err != nil {
			t.Fatalf("unmarshal rendered manifest: %v", err)
		}
		if resource["kind"] != "Deployment" {
			continue
		}

		spec := resource["spec"].(map[string]any)
		template := spec["template"].(map[string]any)
		podSpec := template["spec"].(map[string]any)
		containers := podSpec["containers"].([]any)
		rclone := containers[0].(map[string]any)
		securityContext := rclone["securityContext"].(map[string]any)
		if privileged, _ := securityContext["privileged"].(bool); !privileged {
			t.Fatal("rclone controller must be privileged for Bidirectional mount propagation")
		}
		return
	}

	t.Fatal("rclone controller Deployment was not rendered")
}
