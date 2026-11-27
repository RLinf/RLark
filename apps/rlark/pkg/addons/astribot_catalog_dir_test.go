package addons

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// An item of the catalog directory is offered and rendered like a built-in
// one, in both layouts, and appears without a restart.
func TestCatalogDirectoryAddsItemsAtRuntime(t *testing.T) {
	dir := t.TempDir()
	r := newCatalog(dir)
	builtIn := len(r.List())
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	meta := "name: %s\ndisplayName: Robot\ncategory: robot\nversion: v1\ndescription: d\nparameters:\n- {name: image, displayName: Image, type: string, required: true}\n"
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: profile, namespace: {{ .Namespace }}}\ndata: {image: \"{{ .Values.image }}\"}\n"
	write("fleet-robot-s1u/addon.yaml", strings.Replace(meta, "%s", "fleet-robot-s1u", 1))
	write("fleet-robot-s1u/manifests/profile.yaml", manifest)
	// The shape of a mounted ConfigMap: manifests beside addon.yaml, revisions in "..data".
	write("flat/addon.yaml", strings.Replace(meta, "%s", "flat-item", 1))
	write("flat/profile.yaml", manifest)
	write("flat/..data/ignored.yaml", "not: a manifest")
	// A built-in name: other content for the built-in version is refused, another version is one more item.
	builtInRclone, _ := r.Get("csi-driver-rclone")
	write("clash/addon.yaml", strings.Replace(strings.Replace(meta, "%s", "csi-driver-rclone", 1), "version: v1", "version: "+builtInRclone.Meta().Version, 1))
	write("rclone-before/addon.yaml", strings.Replace(strings.Replace(meta, "%s", "csi-driver-rclone", 1), "version: v1", "version: v0.0.9", 1))
	write("rclone-before/profile.yaml", manifest)
	write("fleet-robot-s1u-next/addon.yaml", strings.Replace(strings.Replace(meta, "%s", "fleet-robot-s1u", 1), "version: v1", "version: v1.10.0", 1))
	write("fleet-robot-s1u-between/addon.yaml", strings.Replace(strings.Replace(meta, "%s", "fleet-robot-s1u", 1), "version: v1", "version: v1.9.0", 1))

	r.readAt = time.Time{} // the directory was read while it was empty; let it be read again
	if got := len(r.List()); got != builtIn+5 {
		t.Fatalf("catalog has %d items, want %d built-in and 5 from the directory", got, builtIn)
	}
	for _, name := range []string{"fleet-robot-s1u", "flat-item"} {
		addon, ok := r.GetVersion(name, "v1")
		if !ok {
			t.Fatalf("%s is not offered", name)
		}
		rendered, err := addon.Render(map[string]string{"image": "registry/robot@sha256:1"}, "rlark-system", name, "uid")
		if err != nil || len(rendered) != 1 || !strings.Contains(string(rendered[0].Raw), "registry/robot@sha256:1") || !strings.Contains(string(rendered[0].Raw), "namespace: rlark-system") {
			t.Fatalf("%s rendered %v %v", name, rendered, err)
		}
	}
	if addon, _ := r.Get("csi-driver-rclone"); addon != builtInRclone {
		t.Fatal("a directory item replaced a built-in one")
	}
	if addon, _ := r.GetVersion("csi-driver-rclone", builtInRclone.Meta().Version); addon != builtInRclone {
		t.Fatal("the directory's content replaced the built-in version's")
	}
	if addon, ok := r.GetVersion("csi-driver-rclone", "v0.0.9"); !ok || len(PackageOf(addon).Files) != 2 {
		t.Fatal("a further version of a built-in item is not offered")
	}
	// The current version of a name that is not built in is its highest, as numbers.
	if addon, _ := r.Get("fleet-robot-s1u"); addon.Meta().Version != "v1.10.0" {
		t.Fatalf("current version %s, want v1.10.0", addon.Meta().Version)
	}
	if _, ok := r.GetVersion("fleet-robot-s1u", "v2"); ok {
		t.Fatal("a version the catalog does not hold was offered")
	}

	// A later item appears once the directory is read again.
	write("later/addon.yaml", strings.Replace(meta, "%s", "later-item", 1))
	r.readAt = time.Time{}
	if _, ok := r.Get("later-item"); !ok {
		t.Fatal("an item added to the directory is not offered")
	}
}

// A Package holds what an item is rendered from, and renders the same after it was published and read back.
func TestPackageRendersLikeItsItem(t *testing.T) {
	for _, meta := range Catalog.List() {
		item, _ := Catalog.Get(meta.Name)
		pkg := PackageOf(item)
		published := pkg.ConfigMap("cluster-a")
		if published.Name != PackageName(meta.Name, meta.Version) || !*published.Immutable || published.Labels[LabelPackage] != meta.Name {
			t.Fatalf("published as %#v", published.ObjectMeta)
		}
		size := 0
		for _, content := range published.Data {
			size += len(content)
		}
		if size > 900_000 {
			t.Fatalf("%s is %d bytes, more than a ConfigMap holds", meta.Name, size)
		}
		read, err := FromPackage(published.Data)
		if err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		for _, parameter := range meta.Parameters {
			if parameter.Required && parameter.Default == "" {
				values[parameter.Name] = "registry.example/image@sha256:1"
			}
		}
		want, wantErr := item.Render(values, "rlark-system", meta.Name, "uid")
		got, gotErr := read.Render(values, "rlark-system", meta.Name, "uid")
		if wantErr != nil || gotErr != nil || len(want) != len(got) {
			t.Fatalf("%s: rendered %d (%v) from the catalog and %d (%v) from the package", meta.Name, len(want), wantErr, len(got), gotErr)
		}
		decodeStrict(t, meta.Name, want)
		for i := range want {
			if string(want[i].Raw) != string(got[i].Raw) {
				t.Fatalf("%s: manifest %d differs", meta.Name, i)
			}
		}
	}
	if PackageName("s1u-robot", "V1.1.0+build 2") != "addon-package-s1u-robot-v1.1.0-build-2" {
		t.Fatal(PackageName("s1u-robot", "V1.1.0+build 2"))
	}
	if _, err := FromPackage(map[string]string{"daemonset.yaml": "kind: DaemonSet"}); err == nil {
		t.Fatal("a package without addon.yaml was accepted")
	}
}

func TestMissingRequired(t *testing.T) {
	meta, err := parseMeta([]byte(`parameters:
- {name: image, required: true}
- {name: mode, required: true, default: auto}
- {name: note}
- {name: device}
- {name: url, requiredWith: device}
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		values map[string]string
		want   string
	}{
		{map[string]string{"image": "i"}, ""},
		{map[string]string{}, "image"},
		{map[string]string{"image": "i", "mode": ""}, "mode"},
		{map[string]string{"image": "i", "device": "d"}, "url"},
		{map[string]string{"image": "i", "device": "d", "url": "u"}, ""},
	} {
		if got := strings.Join(MissingRequired(meta, test.values), ","); got != test.want {
			t.Fatalf("%v: missing %q, want %q", test.values, got, test.want)
		}
	}
}

var (
	strictDecoder = serializer.NewCodecFactory(clientgoscheme.Scheme, serializer.EnableStrict).UniversalDeserializer()
	separator     = regexp.MustCompile(`(?m)^---\s*$`)
	comment       = regexp.MustCompile(`(?m)^\s*#.*$`)
)

// decodeStrict decodes every document rendered as a Kubernetes object, refusing unknown and duplicate fields.
func decodeStrict(t *testing.T, name string, manifests []Manifest) []runtime.Object {
	t.Helper()
	var objects []runtime.Object
	for _, manifest := range manifests {
		for _, document := range separator.Split(string(manifest.Raw), -1) {
			if strings.TrimSpace(comment.ReplaceAllString(document, "")) == "" {
				continue
			}
			object, _, err := strictDecoder.Decode([]byte(document), nil, nil)
			if err != nil {
				t.Fatalf("%s: %v\n%s", name, err, document)
			}
			objects = append(objects, object)
		}
	}
	return objects
}

func render(t *testing.T, name string, values map[string]string) []runtime.Object {
	t.Helper()
	item, _ := Catalog.Get(name)
	manifests, err := item.Render(values, "rlark-system", name, "uid")
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return decodeStrict(t, name, manifests)
}

func containerNames(containers []corev1.Container) []string {
	var names []string
	for _, container := range containers {
		names = append(names, container.Name)
	}
	return names
}

// The live robots' s1u-robot: the device layer, its Station and the Orin payloads, each payload an init container of
// its own.
func TestS1URobotRendersTheDeviceLayerAndEachPayload(t *testing.T) {
	values := map[string]string{
		"image": "registry/rlark/s1u-robot@sha256:1", "deviceImage": "registry/fleet/robot@sha256:2",
		"orinPayloads":      "quest-bridge=registry/fleet/quest-bridge@sha256:3,leds=registry/x/leds@sha256:4",
		"stationControlURL": "http://fleet-dashboard.rlark.internal:33334",
	}
	item, _ := Catalog.Get("s1u-robot")
	missing := MissingRequired(item.Meta(), map[string]string{"image": values["image"], "deviceImage": values["deviceImage"]})
	if !slices.Equal(missing, []string{"stationControlURL"}) {
		t.Fatalf("a device layer without its Station's control URL: missing %v", missing)
	}
	var daemonSet *appsv1.DaemonSet
	for _, object := range render(t, "s1u-robot", values) {
		if d, ok := object.(*appsv1.DaemonSet); ok {
			daemonSet = d
		}
	}
	pod := daemonSet.Spec.Template.Spec
	if got := containerNames(pod.InitContainers); !slices.Equal(got, []string{"device-data", "payload-0", "payload-1"}) {
		t.Fatalf("init containers %v", got)
	}
	names := containerNames(pod.Containers)
	if !slices.Contains(names, "device") || slices.Contains(names, "device-video") || !slices.Contains(names, "station") {
		t.Fatalf("containers %v", names)
	}
	// The Station: the device layer's image, its control URL, a memory limit of its own, its own /dev/shm in memory,
	// the claim's directory read-only and the in-use marks writable; the Pod gives it a minute to leave.
	station := pod.Containers[slices.Index(names, "station")]
	mounts := map[string]bool{}
	for _, mount := range station.VolumeMounts {
		mounts[mount.Name+":"+mount.MountPath] = mount.ReadOnly
	}
	volume := pod.Volumes[slices.IndexFunc(pod.Volumes, func(v corev1.Volume) bool { return v.Name == "station-shm" })]
	if station.Image != values["deviceImage"] || !slices.Equal(station.Command, []string{"/device/start", "--station"}) ||
		station.Env[0].Name != "STATION_CONTROL_URL" || station.Env[0].Value != values["stationControlURL"] ||
		station.Resources.Limits.Memory().String() != "2304Mi" || station.Resources.Requests.Memory().String() != "2Gi" ||
		volume.EmptyDir.Medium != corev1.StorageMediumMemory || volume.EmptyDir.SizeLimit.String() != "512Mi" ||
		!maps.Equal(mounts, map[string]bool{"device-data:/var/lib/fleet": false, "device-run:/run/fleet": false,
			"device-token:/site-token": true, "station-shm:/dev/shm": false, "profile:/etc/rlark/s1u": true,
			"host:/run/rlark/s1u": true, "in-use:/run/rlark/in-use": false}) || *pod.TerminationGracePeriodSeconds < 60 {
		t.Fatalf("station %+v, volume %+v, grace %d", station, volume, *pod.TerminationGracePeriodSeconds)
	}
	// The cameras: served on the node's port the Gateway reaches through the Pod's proxy, ready once it answers; it
	// and the orin container share the directory a USB reset is asked through, and of the host's it reads only whether
	// the robot is in use (a stuck camera's driver is restarted only while it is not).
	video, orin := pod.Containers[slices.Index(names, "video")], pod.Containers[slices.Index(names, "orin")]
	shared := func(c corev1.Container) bool {
		return slices.ContainsFunc(c.VolumeMounts, func(m corev1.VolumeMount) bool { return m.Name == "camera" && m.MountPath == "/camera" })
	}
	inUse := slices.ContainsFunc(video.VolumeMounts, func(m corev1.VolumeMount) bool {
		return m.Name == "in-use" && m.MountPath == "/in-use" && m.ReadOnly
	})
	if video.ReadinessProbe.HTTPGet.Path != "/cameras" || video.ReadinessProbe.HTTPGet.Port.IntValue() != 18891 ||
		len(video.VolumeMounts) != 2 || !shared(video) || !inUse || !shared(orin) || !strings.HasSuffix(video.Command[2], "/s1u/device/cameras.py") {
		t.Fatalf("video %+v", video)
	}
	// The Pod fits what the node profile leaves the Pods, with 256 MiB for the cluster's own Pods: its requests
	// kubelet's allocatable (S1U_KUBERNETES_MEMORY_MIB less the eviction threshold; a Pod that does not fit is never
	// placed, and the robot is left without its power, tuner and device layer), its limits kubepods.slice's
	// memory.max (S1U_KUBERNETES_MEMORY_MIB; past it the kernel ends an arbitrary container of this node-critical
	// Pod, not the one that ran away).
	profile, _ := os.ReadFile("../../../../astribot/s1u/tuner/node_profile.env")
	mib := func(key string) int64 {
		value, _ := strconv.ParseInt(regexp.MustCompile(`(?m)^` + key + `=(\d+)$`).FindStringSubmatch(string(profile))[1], 10, 64)
		return value << 20
	}
	var requested, limited int64
	for _, container := range pod.Containers {
		requested += container.Resources.Requests.Memory().Value()
		limited += container.Resources.Limits.Memory().Value()
	}
	if room := mib("S1U_KUBERNETES_MEMORY_MIB") - mib("S1U_EVICTION_MEMORY_MIB") - 256<<20; requested > room {
		t.Fatalf("the Pod requests %d MiB of memory, the node profile leaves it %d MiB", requested>>20, room>>20)
	}
	if room := mib("S1U_KUBERNETES_MEMORY_MIB") - 256<<20; limited > room {
		t.Fatalf("the Pod's memory limits are %d MiB, the node profile leaves it %d MiB", limited>>20, room>>20)
	}
}

// site-registry in the roles the site runs: the registry on a local volume beside its loopback entry, and the
// pull-through cache with its prewarm.
func TestSiteRegistryRendersTheSitesRoles(t *testing.T) {
	base := map[string]string{"image": "registry@sha256:x", "relayImage": "rlark@sha256:y", "listen": "0.0.0.0:5443"}
	for _, test := range []struct {
		values      map[string]string
		deployments int
		containers  int
	}{
		{map[string]string{"storage": "filesystem", "volumeSize": "300G", "localListen": "127.0.0.1:5443"}, 1, 2},
		{map[string]string{"role": "cache", "upstream": "https://origin.rlark.internal:5443", "caCert": "x", "prewarm": "registry.rlark.internal:5443/fleet/robot@sha256:z"}, 2, 2},
	} {
		values := maps.Clone(base)
		maps.Copy(values, test.values)
		var deployments []*appsv1.Deployment
		for _, object := range render(t, "site-registry", values) {
			if d, ok := object.(*appsv1.Deployment); ok {
				deployments = append(deployments, d)
			}
		}
		if len(deployments) != test.deployments || len(deployments[len(deployments)-1].Spec.Template.Spec.Containers) != test.containers {
			t.Fatalf("%v: %d Deployments", test.values, len(deployments))
		}
	}
}
