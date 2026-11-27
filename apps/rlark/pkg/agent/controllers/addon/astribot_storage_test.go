package addon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

const storageAddonManifest = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: reader
spec:
  selector:
    matchLabels: {app: reader}
  template:
    metadata:
      labels: {app: reader}
    spec:
      containers:
        - name: main
          image: busybox
          volumeMounts:
            - {name: data, mountPath: /data, subPath: rl_data}
      volumes:
        - name: data
          csi:
            driver: storage.rlark.io
            volumeAttributes: {storage: kpfs, profile: read}
`

func storageAddonReconciler(t *testing.T, views ...storageref.View) *pullReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, view := range views {
		data, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		builder = builder.WithObjects(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-" + view.Name + "-storage", Namespace: "rlark-ksyun-gpu", Labels: map[string]string{storageref.ViewLabel: "true"}},
			Data:       map[string]string{storageref.ViewKey: string(data)},
		})
	}
	return &pullReconciler{c: &Controller{Controller: base.Controller{ManagementClient: builder.Build()}}}
}

func TestAddonStorageVolumeResolvesFromTheClusterViews(t *testing.T) {
	r := storageAddonReconciler(t, storageref.View{Name: "kpfs", Type: storageref.TypeHostPath, StorageClasses: map[string]string{"default": "sc-cccccccccccccccc", "read": "sc-cccccccccccccccc-read"}, NodeSelector: map[string]string{"rlark.io/kpfs": "true"}})
	objects, err := parseManifest([]byte(storageAddonManifest))
	if err != nil {
		t.Fatal(err)
	}
	var views []storageref.View
	if err := r.resolveStorage(context.Background(), objects[0], "rlark-ksyun-gpu", &views); err != nil {
		t.Fatal(err)
	}
	spec, _, err := storageref.PodSpecOf(objects[0])
	if err != nil {
		t.Fatal(err)
	}
	if spec.Volumes[0].Ephemeral == nil || *spec.Volumes[0].Ephemeral.VolumeClaimTemplate.Spec.StorageClassName != "sc-cccccccccccccccc-read" || spec.Volumes[0].CSI != nil {
		t.Fatalf("volume = %+v", spec.Volumes[0].VolumeSource)
	}
	mount := spec.Containers[0].VolumeMounts[0]
	if !mount.ReadOnly || mount.SubPath != "rl_data" {
		t.Fatalf("mount = %+v, want read-only rl_data", mount)
	}
	if spec.Affinity == nil {
		t.Fatal("no node affinity to the nodes that have the storage")
	}
}

func TestAddonStorageVolumeNamingAMissingStorageFails(t *testing.T) {
	r := storageAddonReconciler(t)
	objects, err := parseManifest([]byte(storageAddonManifest))
	if err != nil {
		t.Fatal(err)
	}
	var views []storageref.View
	err = r.resolveStorage(context.Background(), objects[0], "rlark-ksyun-gpu", &views)
	if err == nil || !strings.Contains(err.Error(), `DaemonSet reader: volume "data": storage "kpfs" is not available in cluster "rlark-ksyun-gpu"`) {
		t.Fatalf("err = %v", err)
	}
}
