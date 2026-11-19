package nodepath

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func storageClass(name, id, root, fsType string) *storagev1.StorageClass {
	return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{StorageIDLabel: id}},
		Provisioner: Name, Parameters: map[string]string{ParameterRoot: root, ParameterFSType: fsType}}
}

func events(t *testing.T, client *fake.Clientset, namespace string) []string {
	t.Helper()
	list, err := client.CoreV1().Events(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for _, event := range list.Items {
		result = append(result, event.InvolvedObject.Kind+"/"+event.Reason+": "+event.Message)
	}
	return result
}

func TestProbeLabelsTheNodeWithTheStoragesItHas(t *testing.T) {
	node := newFakeNode(t)
	client := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"storage.rlark.io/sc-gone": "true", "zone": "a"}}},
		storageClass("sc-kpfs", "sc-kpfs", "/mnt/xc_kpfs/regular", "kingstorage"),
		storageClass("sc-kpfs-read", "sc-kpfs", "/mnt/xc_kpfs/regular", "kingstorage"),
		storageClass("sc-other", "sc-other", "/mnt/other", ""),
	)
	prober := &Prober{Client: client, Node: "node-a", HostRoot: node.host, Mounts: node.table, Published: node.records, Now: time.Now}
	if err := prober.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := client.CoreV1().Nodes().Get(context.Background(), "node-a", metav1.GetOptions{})
	if got.Labels[NodeLabel("sc-kpfs")] != "true" || got.Labels["zone"] != "a" {
		t.Fatalf("labels = %v", got.Labels)
	}
	if _, found := got.Labels["storage.rlark.io/sc-gone"]; found {
		t.Fatal("the label of a storage that no longer exists stayed")
	}
	if _, found := got.Labels[NodeLabel("sc-other")]; found {
		t.Fatal("a storage the node lacks is labelled")
	}
	nodeEvents := events(t, client, "default")
	if len(nodeEvents) != 1 || !strings.Contains(nodeEvents[0], "Node/StorageUnavailable: storage sc-other: node node-a has no directory /mnt/other") {
		t.Fatalf("events = %v", nodeEvents)
	}

	// KPFS goes away: the label goes and the change is an event; a second
	// probe with nothing new adds nothing.
	node.mounts = node.mounts[:2]
	for range 2 {
		if err := prober.Probe(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	got, _ = client.CoreV1().Nodes().Get(context.Background(), "node-a", metav1.GetOptions{})
	if _, found := got.Labels[NodeLabel("sc-kpfs")]; found {
		t.Fatal("the label stayed after the storage's mount went away")
	}
	nodeEvents = events(t, client, "default")
	if len(nodeEvents) != 2 || !strings.Contains(nodeEvents[1], "is not on a mounted file system") {
		t.Fatalf("events = %v", nodeEvents)
	}
}

func TestProbeTellsAPodWhoseMountWasRemounted(t *testing.T) {
	node := newFakeNode(t)
	d := node.driver()
	target := filepath.Join(t.TempDir(), "t")
	if _, err := d.NodePublishVolume(context.Background(), publishRequest(target, map[string]string{
		ParameterRoot: "/mnt/xc_kpfs/regular", ParameterFSType: "kingstorage", podNamespaceKey: "rlark-system", podNameKey: "job-c1-0"})); err != nil {
		t.Fatal(err)
	}
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}, storageClass("sc-kpfs", "sc-kpfs", "/mnt/xc_kpfs/regular", "kingstorage"))
	prober := &Prober{Client: client, Node: "node-a", HostRoot: node.host, Mounts: node.table, Published: node.records, Now: time.Now}
	if err := prober.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if podEvents := events(t, client, "rlark-system"); len(podEvents) != 0 {
		t.Fatalf("events before any remount: %v", podEvents)
	}
	// The node remounts KPFS: a new superblock. The Pod's bind keeps the old.
	node.mounts[2].Device = "0:99"
	for range 2 {
		if err := prober.Probe(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	podEvents := events(t, client, "rlark-system")
	if len(podEvents) != 1 || !strings.Contains(podEvents[0], "Pod/StorageRemounted: the node's mount of /mnt/xc_kpfs/regular changed after this Pod bound it (device 0:45, now device 0:99)") {
		t.Fatalf("pod events = %v", podEvents)
	}
	if nodeEvents := events(t, client, "default"); len(nodeEvents) != 1 || !strings.Contains(nodeEvents[0], "Node/StorageRemounted: pod rlark-system/job-c1-0") {
		t.Fatalf("node events = %v", nodeEvents)
	}
}
