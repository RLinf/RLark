package addons

import (
	"bytes"
	"io"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRcloneManifestsUseSafeMountsAndCanarySelector(t *testing.T) {
	addon, ok := Registry.Get("csi-driver-rclone")
	if !ok {
		t.Fatal("csi-driver-rclone addon is not registered")
	}

	manifests, err := addon.Render(map[string]string{
		"nodeSelector": "rlark.io/rclone-csi=enabled",
	}, "rlark-system", "csi-driver-rclone", "test-uid")
	if err != nil {
		t.Fatalf("render addon: %v", err)
	}

	var foundController, foundNode, foundLeasePermissions bool
	for _, manifest := range manifests {
		decoder := yaml.NewDecoder(bytes.NewReader(manifest.Raw))
		for {
			var resource map[string]any
			if err := decoder.Decode(&resource); err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("decode rendered manifest: %v", err)
			}
			if len(resource) == 0 {
				continue
			}

			switch resource["kind"] {
			case "Deployment":
				foundController = true
				spec := resource["spec"].(map[string]any)
				template := spec["template"].(map[string]any)
				podSpec := template["spec"].(map[string]any)
				containers := podSpec["containers"].([]any)
				provisioner := containers[1].(map[string]any)
				for _, arg := range provisioner["args"].([]any) {
					if arg == "--feature-gates=HonorPVReclaimPolicy=true" {
						t.Fatal("csi-provisioner v6.2.0 does not support HonorPVReclaimPolicy")
					}
				}
				for _, volume := range podSpec["volumes"].([]any) {
					if _, ok := volume.(map[string]any)["hostPath"]; ok {
						t.Fatalf("rclone controller must not mount host paths: %v", volume)
					}
				}
			case "ClusterRole":
				for _, rule := range resource["rules"].([]any) {
					rule := rule.(map[string]any)
					apiGroups := rule["apiGroups"].([]any)
					resources := rule["resources"].([]any)
					if len(apiGroups) == 1 && apiGroups[0] == "coordination.k8s.io" && len(resources) == 1 && resources[0] == "leases" {
						verbs := make(map[any]bool)
						for _, verb := range rule["verbs"].([]any) {
							verbs[verb] = true
						}
						for _, verb := range []string{"get", "list", "watch", "create", "update", "patch"} {
							if !verbs[verb] {
								t.Fatalf("missing lease permission %q: %v", verb, rule)
							}
						}
						foundLeasePermissions = true
					}
				}
			case "DaemonSet":
				foundNode = true
				spec := resource["spec"].(map[string]any)
				rollingUpdate := spec["updateStrategy"].(map[string]any)["rollingUpdate"].(map[string]any)
				if rollingUpdate["maxUnavailable"] != 1 {
					t.Fatalf("unexpected maxUnavailable: %v", rollingUpdate["maxUnavailable"])
				}
				template := spec["template"].(map[string]any)
				podSpec := template["spec"].(map[string]any)
				nodeSelector := podSpec["nodeSelector"].(map[string]any)
				if nodeSelector["rlark.io/rclone-csi"] != "enabled" {
					t.Fatalf("unexpected nodeSelector: %v", nodeSelector)
				}
				for _, volume := range podSpec["volumes"].([]any) {
					volume := volume.(map[string]any)
					hostPath, ok := volume["hostPath"].(map[string]any)
					if !ok {
						continue
					}
					if hostPath["path"] == "/var/lib/kubelet" {
						t.Fatal("rclone node must not mount the kubelet root directory")
					}
				}
			}
		}
	}

	if !foundController || !foundNode || !foundLeasePermissions {
		t.Fatalf("missing rendered resources: controller=%v node=%v leasePermissions=%v", foundController, foundNode, foundLeasePermissions)
	}
}
