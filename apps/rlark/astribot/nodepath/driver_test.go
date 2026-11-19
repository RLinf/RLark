package nodepath

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeNode is a node's file systems as the plugin sees them: a host root
// directory with KPFS "mounted" at /mnt/xc_kpfs, and the binds the driver made.
type fakeNode struct {
	host    string
	mounts  []Mount
	ro      map[string]bool
	records *PublishedStore
}

func (n *fakeNode) BindMount(source, target string, readOnly bool) error {
	device := ""
	if mount, found := covering(n.mounts, source); found {
		device = mount.Device
	}
	n.mounts = append(n.mounts, Mount{Device: device, MountPoint: target, FSType: "bind"})
	n.ro[target] = readOnly
	return nil
}

func (n *fakeNode) Unmount(target string) error {
	kept := n.mounts[:0]
	for _, mount := range n.mounts {
		if mount.MountPoint != target {
			kept = append(kept, mount)
		}
	}
	n.mounts = kept
	return nil
}

func (n *fakeNode) table() ([]Mount, error) { return append([]Mount(nil), n.mounts...), nil }

func newFakeNode(t *testing.T) *fakeNode {
	t.Helper()
	host := t.TempDir()
	if err := os.MkdirAll(filepath.Join(host, "mnt/xc_kpfs/regular"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &fakeNode{
		host: host,
		mounts: []Mount{
			{Device: "8:1", MountPoint: "/", FSType: "overlay"},
			{Device: "253:0", MountPoint: host, FSType: "ext4"},
			{Device: "0:45", MountPoint: filepath.Join(host, "mnt/xc_kpfs"), FSType: "kingstorage"},
		},
		ro:      map[string]bool{},
		records: &PublishedStore{Dir: t.TempDir()},
	}
}

func (n *fakeNode) driver() *Driver {
	return &Driver{NodeID: "node-a", HostRoot: n.host, Mounter: n, Mounts: n.table, Published: n.records}
}

func publishRequest(target string, volumeContext map[string]string) *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{VolumeId: "pvc-1", TargetPath: target, VolumeContext: volumeContext,
		VolumeCapability: &csi.VolumeCapability{AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}}}}
}

func TestCreateVolumeCarriesTheRoot(t *testing.T) {
	d := newFakeNode(t).driver()
	response, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "pvc-1", Parameters: map[string]string{ParameterRoot: "/mnt/xc_kpfs/regular", ParameterFSType: "kingstorage", ParameterReadOnly: "true", "csi.storage.k8s.io/pvc/name": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	volumeContext := response.Volume.VolumeContext
	if response.Volume.VolumeId != "pvc-1" || volumeContext[ParameterRoot] != "/mnt/xc_kpfs/regular" || volumeContext[ParameterReadOnly] != "true" || volumeContext[ParameterFSType] != "kingstorage" {
		t.Fatalf("volume = %+v", response.Volume)
	}
	for _, parameters := range []map[string]string{
		{ParameterRoot: "relative"}, {ParameterRoot: "/"}, {ParameterRoot: "/a/../b"}, {ParameterRoot: "/a", "other": "x"}, {ParameterRoot: "/a", ParameterReadOnly: "yes"},
	} {
		if _, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{Name: "pvc", Parameters: parameters}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("parameters %v: err = %v, want InvalidArgument", parameters, err)
		}
	}
}

func TestPublishBindsTheNodesMountAndRecordsIt(t *testing.T) {
	node := newFakeNode(t)
	d := node.driver()
	target := filepath.Join(t.TempDir(), "pods/x/volumes/data")
	request := publishRequest(target, map[string]string{ParameterRoot: "/mnt/xc_kpfs/regular", ParameterFSType: "kingstorage",
		podNamespaceKey: "rlark-system", podNameKey: "job-c1-0"})
	if _, err := d.NodePublishVolume(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !mountedAt(node.mounts, target) || node.ro[target] {
		t.Fatalf("mounts = %v ro = %v", node.mounts, node.ro)
	}
	records, err := node.records.List()
	if err != nil || len(records) != 1 || records[0].Device != "0:45" || records[0].PodName != "job-c1-0" {
		t.Fatalf("records = %+v, %v", records, err)
	}
	// Publishing again does nothing more.
	if _, err := d.NodePublishVolume(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(strings.Join(mountPoints(node.mounts), " "), target); count != 1 {
		t.Fatalf("target mounted %d times", count)
	}
	if _, err := d.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{VolumeId: "pvc-1", TargetPath: target}); err != nil {
		t.Fatal(err)
	}
	if mountedAt(node.mounts, target) {
		t.Fatal("target still mounted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target left behind: %v", err)
	}
	if records, _ := node.records.List(); len(records) != 0 {
		t.Fatalf("records left: %+v", records)
	}
	// Unpublishing an absent target succeeds.
	if _, err := d.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{VolumeId: "pvc-1", TargetPath: target}); err != nil {
		t.Fatal(err)
	}
}

func mountPoints(mounts []Mount) []string {
	points := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		points = append(points, mount.MountPoint)
	}
	return points
}

func TestPublishReadOnlyFromProfileOrPod(t *testing.T) {
	node := newFakeNode(t)
	d := node.driver()
	for name, request := range map[string]*csi.NodePublishVolumeRequest{
		"profile": publishRequest(filepath.Join(t.TempDir(), "p"), map[string]string{ParameterRoot: "/mnt/xc_kpfs/regular", ParameterReadOnly: "true"}),
		"pod":     publishRequest(filepath.Join(t.TempDir(), "q"), map[string]string{ParameterRoot: "/mnt/xc_kpfs/regular"}),
	} {
		request.Readonly = name == "pod"
		if _, err := d.NodePublishVolume(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if !node.ro[request.TargetPath] {
			t.Fatalf("%s: mount is writable", name)
		}
	}
}

func TestPublishRefusesARootThatIsNotMounted(t *testing.T) {
	for name, test := range map[string]struct {
		root, fsType string
		mounts       func(host string) []Mount
		want         string
	}{
		"no directory": {"/mnt/st3-does-not-exist", "", nil, "node node-a has no directory /mnt/st3-does-not-exist"},
		// KPFS did not mount: the directory is there, on the node's own disk.
		"directory on the node's disk": {"/mnt/xc_kpfs/regular", "kingstorage", func(host string) []Mount {
			return []Mount{{Device: "253:0", MountPoint: host, FSType: "ext4"}}
		}, "node node-a: /mnt/xc_kpfs/regular is not on a mounted file system (it is on the node's root file system), expected kingstorage"},
		"another file system": {"/mnt/xc_kpfs/regular", "kingstorage", func(host string) []Mount {
			return []Mount{{Device: "253:0", MountPoint: host, FSType: "ext4"}, {Device: "0:50", MountPoint: filepath.Join(host, "mnt/xc_kpfs"), FSType: "tmpfs"}}
		}, "node node-a: /mnt/xc_kpfs is mounted as tmpfs, expected kingstorage"},
	} {
		t.Run(name, func(t *testing.T) {
			node := newFakeNode(t)
			if test.mounts != nil {
				node.mounts = test.mounts(node.host)
			}
			_, err := node.driver().NodePublishVolume(context.Background(), publishRequest(filepath.Join(t.TempDir(), "t"),
				map[string]string{ParameterRoot: test.root, ParameterFSType: test.fsType}))
			if status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseMountInfo(t *testing.T) {
	mounts := ParseMountInfo(`4250 4234 0:45 /regular /mnt/xc_kpfs/regular ro,noatime master:298 - kingstorage NAS-NXLT01-ROCE-PUBLIC01_xc_kpfs rw
22 1 253:0 / /host rw,relatime shared:1 - ext4 /dev/vda1 rw
30 22 0:60 / /host/mnt/with\040space rw - tmpfs tmpfs rw
`)
	if len(mounts) != 3 || mounts[0] != (Mount{Device: "0:45", MountPoint: "/mnt/xc_kpfs/regular", FSType: "kingstorage"}) || mounts[2].MountPoint != "/host/mnt/with space" {
		t.Fatalf("mounts = %+v", mounts)
	}
	if mount, _ := covering(mounts, "/host/mnt/with space/a"); mount.FSType != "tmpfs" {
		t.Fatalf("covering = %+v", mount)
	}
	if mount, _ := covering(mounts, "/host/mnt/with spaceX"); mount.FSType != "ext4" {
		t.Fatalf("a sibling with a common prefix is not covered: %+v", mount)
	}
}
