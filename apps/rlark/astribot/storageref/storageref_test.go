package storageref

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var testViews = []View{
	{Name: "ks3", Type: TypeS3, DefaultProfile: ProfileArchive, StorageClasses: map[string]string{
		ProfileArchive: "sc-0123456789abcdef", ProfileDefault: "sc-0123456789abcdef-default", ProfileRead: "sc-0123456789abcdef-read",
	}},
	{Name: "nas", Type: TypeSMB, DefaultProfile: ProfileDefault, StorageClasses: map[string]string{
		ProfileDefault: "sc-1111111111111111", ProfileRead: "sc-1111111111111111-read",
	}},
	{Name: "kpfs", Type: TypeHostPath, DefaultProfile: ProfileDefault, StorageClasses: map[string]string{"default": "sc-cccccccccccccccc", "read": "sc-cccccccccccccccc-read"}, NodeSelector: map[string]string{"rlark.io/kpfs": "true"}},
}

func reference(name, storage, profile string, readOnly bool) corev1.Volume {
	attributes := map[string]string{AttributeStorage: storage}
	if profile != "" {
		attributes[AttributeProfile] = profile
	}
	source := &corev1.CSIVolumeSource{Driver: Driver, VolumeAttributes: attributes}
	if readOnly {
		source.ReadOnly = ptr.To(true)
	}
	return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{CSI: source}}
}

func podSpec(volumes ...corev1.Volume) *corev1.PodSpec {
	mounts := make([]corev1.VolumeMount, 0, len(volumes))
	for _, volume := range volumes {
		mounts = append(mounts, corev1.VolumeMount{Name: volume.Name, MountPath: "/mnt/" + volume.Name, SubPath: "fleet"})
	}
	return &corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "init", VolumeMounts: mounts}},
		Containers:     []corev1.Container{{Name: "main", VolumeMounts: append([]corev1.VolumeMount(nil), mounts...)}},
		Volumes:        volumes,
	}
}

func TestResolveObjectStorageUsesTheProfileClass(t *testing.T) {
	spec := podSpec(reference("default", "ks3", "", false), reference("read", "ks3", ProfileRead, false), reference("plain", "ks3", ProfileDefault, false))
	if err := Resolve(spec, testViews, "rlark-ksyun-gpu"); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"sc-0123456789abcdef", "sc-0123456789abcdef-read", "sc-0123456789abcdef-default"} {
		ephemeral := spec.Volumes[i].Ephemeral
		if ephemeral == nil || spec.Volumes[i].CSI != nil {
			t.Fatalf("volume %d = %+v, want only an ephemeral claim", i, spec.Volumes[i].VolumeSource)
		}
		if got := *ephemeral.VolumeClaimTemplate.Spec.StorageClassName; got != want {
			t.Fatalf("volume %d class = %q, want %q", i, got, want)
		}
	}
	for _, container := range [][]corev1.Container{spec.InitContainers, spec.Containers} {
		mounts := container[0].VolumeMounts
		if mounts[0].ReadOnly || !mounts[1].ReadOnly || mounts[2].ReadOnly {
			t.Fatalf("read-only mounts = %v %v %v, want only the read profile", mounts[0].ReadOnly, mounts[1].ReadOnly, mounts[2].ReadOnly)
		}
		if mounts[1].SubPath != "fleet" {
			t.Fatalf("subPath was changed: %q", mounts[1].SubPath)
		}
	}
}

func TestResolveReadOnlyVolumeOnAnyProfile(t *testing.T) {
	spec := podSpec(reference("nas", "nas", "", true))
	if err := Resolve(spec, testViews, "c"); err != nil {
		t.Fatal(err)
	}
	if got := *spec.Volumes[0].Ephemeral.VolumeClaimTemplate.Spec.StorageClassName; got != "sc-1111111111111111" {
		t.Fatalf("class = %q", got)
	}
	if !spec.Containers[0].VolumeMounts[0].ReadOnly || !spec.InitContainers[0].VolumeMounts[0].ReadOnly {
		t.Fatal("readOnly volume left a writable mount")
	}
}

func TestResolveNodeSelectorAddsNodeAffinityToEveryTerm(t *testing.T) {
	spec := podSpec(reference("kpfs", "kpfs", "", false))
	spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"}}}},
			{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"b"}}}},
		},
	}}}
	if err := Resolve(spec, testViews, "c"); err != nil {
		t.Fatal(err)
	}
	// A host-path storage is a claim like any other storage: no hostPath.
	if spec.Volumes[0].HostPath != nil || *spec.Volumes[0].Ephemeral.VolumeClaimTemplate.Spec.StorageClassName != "sc-cccccccccccccccc" {
		t.Fatalf("volume = %+v", spec.Volumes[0].VolumeSource)
	}
	for _, term := range spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		if len(term.MatchExpressions) != 2 || term.MatchExpressions[1].Key != "rlark.io/kpfs" {
			t.Fatalf("term = %+v, want the zone and the storage's node label", term)
		}
	}
	// Resolving again (a re-render of the same template) adds nothing twice.
	again := podSpec(reference("kpfs", "kpfs", "", false))
	again.Affinity = spec.Affinity.DeepCopy()
	if err := Resolve(again, testViews, "c"); err != nil {
		t.Fatal(err)
	}
	if got := len(again.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions); got != 2 {
		t.Fatalf("expressions = %d after a second resolve, want 2", got)
	}
}

func TestResolveHostPathWithoutSelectorAddsNoAffinity(t *testing.T) {
	spec := podSpec(reference("kpfs", "kpfs", ProfileRead, false))
	views := []View{{Name: "kpfs", Type: TypeHostPath, StorageClasses: map[string]string{"default": "sc-cccccccccccccccc", "read": "sc-cccccccccccccccc-read"}}}
	if err := Resolve(spec, views, "c"); err != nil {
		t.Fatal(err)
	}
	if spec.Affinity != nil {
		t.Fatalf("affinity = %+v, want none", spec.Affinity)
	}
	if !spec.Containers[0].VolumeMounts[0].ReadOnly {
		t.Fatal("read profile on a host path did not make the mount read-only")
	}
}

func TestResolveRejects(t *testing.T) {
	for name, test := range map[string]struct {
		volume corev1.Volume
		want   string
	}{
		"unknown storage":     {reference("x", "fleet-ks3-archive", "", false), `storage "fleet-ks3-archive" is not available in cluster "rlark-ksyun-gpu"`},
		"profile it lacks":    {reference("x", "nas", ProfileArchive, false), `has no profile "archive" (it has default, read)`},
		"host path archive":   {reference("x", "kpfs", ProfileArchive, false), `has no profile "archive" (it has default, read)`},
		"no storage":          {reference("x", "", "", false), "must name a storage"},
		"unknown attribute":   {corev1.Volume{Name: "x", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: Driver, VolumeAttributes: map[string]string{AttributeStorage: "ks3", "subPath": "a"}}}}, `unknown volume attribute "subPath"`},
		"node publish secret": {corev1.Volume{Name: "x", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: Driver, VolumeAttributes: map[string]string{AttributeStorage: "ks3"}, NodePublishSecretRef: &corev1.LocalObjectReference{Name: "s"}}}}, "takes only volumeAttributes"},
	} {
		t.Run(name, func(t *testing.T) {
			err := Resolve(podSpec(test.volume), testViews, "rlark-ksyun-gpu")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveLeavesOtherVolumes(t *testing.T) {
	other := corev1.Volume{Name: "shm", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory}}}
	otherCSI := corev1.Volume{Name: "csi", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: "other.csi.io"}}}
	spec := podSpec(other, otherCSI)
	if HasReferences(spec) {
		t.Fatal("HasReferences = true for a spec without storage volumes")
	}
	if err := Resolve(spec, nil, "c"); err != nil {
		t.Fatal(err)
	}
	if spec.Volumes[0].EmptyDir == nil || spec.Volumes[1].CSI.Driver != "other.csi.io" {
		t.Fatalf("volumes changed: %+v", spec.Volumes)
	}
}
