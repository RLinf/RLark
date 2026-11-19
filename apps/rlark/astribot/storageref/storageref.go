// Package storageref resolves volumes that name an RLark storage into the
// volume the storage is mounted with in one cluster.
//
// A Job or Addon names a storage instead of a generated StorageClass or a node
// path:
//
//	volumes:
//	  - name: data
//	    csi:
//	      driver: storage.rlark.io
//	      readOnly: true            # optional, every mount of the volume
//	      volumeAttributes:
//	        storage: ks3            # the storage's name
//	        profile: read           # optional mount profile
//
// and mounts a directory of it with the container's native volumeMounts
// (subPath). storage.rlark.io is not a CSI driver that runs anywhere: RLark
// rewrites the volume before the Pod reaches Kubernetes, from the View the
// storage has in the cluster the Pod runs in: a generic ephemeral volume of
// the StorageClass the storage has for the chosen profile, whatever its type
// (rclone's for object storage and SMB, the node directory driver's for a
// host-path storage such as Kingsoft Cloud KPFS). A storage that only some
// nodes have also gives the Pod a node affinity to them.
package storageref

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// Driver is the CSI driver name a volume uses to name a storage.
	Driver = "storage.rlark.io"
	// AttributeStorage and AttributeProfile are the volume attributes.
	AttributeStorage = "storage"
	AttributeProfile = "profile"

	// ViewLabel marks the ConfigMap that holds a storage's View in a cluster's
	// namespace of the management plane.
	ViewLabel = "rlark.io/storage-view"
	// ViewKey is the ConfigMap key holding the View as JSON.
	ViewKey = "storage"

	TypeS3       = "s3"
	TypeSMB      = "smb"
	TypeHostPath = "hostpath"

	ProfileDefault = "default"
	ProfileRead    = "read"
	ProfileArchive = "archive"

	// claimSize is the requested size of a claim. Neither driver enforces
	// it; Kubernetes requires one.
	claimSize = "10Gi"
)

// View is what a cluster needs to mount one storage. The Gateway renders it
// from the storage's declaration; it carries no credentials.
type View struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// DefaultProfile is the profile of a volume that names none.
	DefaultProfile string `json:"defaultProfile,omitempty"`
	// StorageClasses maps each profile the storage offers to the StorageClass
	// delivered to the cluster for it.
	StorageClasses map[string]string `json:"storageClasses,omitempty"`
	// NodeSelector selects the nodes that have the storage (a host-path
	// storage's directory); empty means all nodes of the cluster.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// ViewConfigMapName is the name of a storage's View ConfigMap.
func ViewConfigMapName(storageClassID string) string { return storageClassID + "-storage" }

// DecodeView reads a View ConfigMap.
func DecodeView(configMap *corev1.ConfigMap) (View, error) {
	var view View
	if err := json.Unmarshal([]byte(configMap.Data[ViewKey]), &view); err != nil {
		return View{}, fmt.Errorf("decode storage view %s/%s: %w", configMap.Namespace, configMap.Name, err)
	}
	return view, nil
}

// Profiles returns the mount profiles a storage of the type supports.
func Profiles(storageType string) []string {
	switch storageType {
	case TypeS3:
		return []string{ProfileDefault, ProfileRead, ProfileArchive}
	case TypeSMB, TypeHostPath:
		return []string{ProfileDefault, ProfileRead}
	}
	return nil
}

// IsReference reports whether a volume names a storage.
func IsReference(volume corev1.Volume) bool {
	return volume.CSI != nil && volume.CSI.Driver == Driver
}

// HasReferences reports whether a Pod spec has a volume naming a storage.
func HasReferences(spec *corev1.PodSpec) bool {
	for _, volume := range spec.Volumes {
		if IsReference(volume) {
			return true
		}
	}
	return false
}

// Resolve rewrites every volume of spec that names a storage, using the
// Views of the cluster the Pod runs in. cluster only words the errors.
func Resolve(spec *corev1.PodSpec, views []View, cluster string) error {
	byName := make(map[string]View, len(views))
	for _, view := range views {
		byName[view.Name] = view
	}
	for i := range spec.Volumes {
		volume := &spec.Volumes[i]
		if !IsReference(*volume) {
			continue
		}
		name := strings.TrimSpace(volume.CSI.VolumeAttributes[AttributeStorage])
		if name == "" {
			return fmt.Errorf("volume %q: volumeAttributes.%s must name a storage", volume.Name, AttributeStorage)
		}
		for key := range volume.CSI.VolumeAttributes {
			if key != AttributeStorage && key != AttributeProfile {
				return fmt.Errorf("volume %q: unknown volume attribute %q", volume.Name, key)
			}
		}
		if volume.CSI.FSType != nil || volume.CSI.NodePublishSecretRef != nil {
			return fmt.Errorf("volume %q: a storage volume takes only volumeAttributes and readOnly", volume.Name)
		}
		view, found := byName[name]
		if !found {
			return fmt.Errorf("volume %q: storage %q is not available in cluster %q", volume.Name, name, cluster)
		}
		profile := strings.TrimSpace(volume.CSI.VolumeAttributes[AttributeProfile])
		if profile == "" {
			profile = view.DefaultProfile
		}
		if profile == "" {
			profile = ProfileDefault
		}
		readOnly := (volume.CSI.ReadOnly != nil && *volume.CSI.ReadOnly) || profile == ProfileRead
		class, found := view.StorageClasses[profile]
		if !found {
			return fmt.Errorf("volume %q: storage %q has no profile %q (it has %s)", volume.Name, name, profile, strings.Join(slices.Sorted(maps.Keys(view.StorageClasses)), ", "))
		}
		volume.VolumeSource = corev1.VolumeSource{Ephemeral: &corev1.EphemeralVolumeSource{
			VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &class,
				Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(claimSize)}},
			}},
		}}
		requireNodes(spec, view.NodeSelector)
		if readOnly {
			setReadOnly(spec, volume.Name)
		}
	}
	return nil
}

// setReadOnly makes every mount of the volume read-only.
func setReadOnly(spec *corev1.PodSpec, volume string) {
	for _, containers := range [][]corev1.Container{spec.InitContainers, spec.Containers} {
		for i := range containers {
			for j := range containers[i].VolumeMounts {
				if containers[i].VolumeMounts[j].Name == volume {
					containers[i].VolumeMounts[j].ReadOnly = true
				}
			}
		}
	}
}

// requireNodes adds the labels to every term of the Pod's required node
// affinity, so that the Pod lands only on nodes that have them.
func requireNodes(spec *corev1.PodSpec, selector map[string]string) {
	if len(selector) == 0 {
		return
	}
	requirements := make([]corev1.NodeSelectorRequirement, 0, len(selector))
	for _, key := range slices.Sorted(maps.Keys(selector)) {
		requirements = append(requirements, corev1.NodeSelectorRequirement{Key: key, Operator: corev1.NodeSelectorOpIn, Values: []string{selector[key]}})
	}
	if spec.Affinity == nil {
		spec.Affinity = &corev1.Affinity{}
	}
	if spec.Affinity.NodeAffinity == nil {
		spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	required := spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		required = &corev1.NodeSelector{}
		spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = required
	}
	if len(required.NodeSelectorTerms) == 0 {
		required.NodeSelectorTerms = []corev1.NodeSelectorTerm{{}}
	}
	for i := range required.NodeSelectorTerms {
		term := &required.NodeSelectorTerms[i]
		for _, requirement := range requirements {
			if !containsRequirement(term.MatchExpressions, requirement) {
				term.MatchExpressions = append(term.MatchExpressions, requirement)
			}
		}
	}
}

func containsRequirement(requirements []corev1.NodeSelectorRequirement, wanted corev1.NodeSelectorRequirement) bool {
	for _, requirement := range requirements {
		if requirement.Key == wanted.Key && requirement.Operator == wanted.Operator && len(requirement.Values) == 1 && requirement.Values[0] == wanted.Values[0] {
			return true
		}
	}
	return false
}

// podSpecPaths locates the Pod spec in the workload kinds an Addon renders.
var podSpecPaths = map[string][]string{
	"Pod":         {"spec"},
	"Deployment":  {"spec", "template", "spec"},
	"DaemonSet":   {"spec", "template", "spec"},
	"StatefulSet": {"spec", "template", "spec"},
	"ReplicaSet":  {"spec", "template", "spec"},
	"Job":         {"spec", "template", "spec"},
	"CronJob":     {"spec", "jobTemplate", "spec", "template", "spec"},
}

// PodSpecOf returns the Pod spec of a rendered workload object and where it
// is, or a nil spec for an object that has none.
func PodSpecOf(object *unstructured.Unstructured) (*corev1.PodSpec, []string, error) {
	path, found := podSpecPaths[object.GetKind()]
	if !found {
		return nil, nil, nil
	}
	raw, found, err := unstructured.NestedMap(object.Object, path...)
	if err != nil || !found {
		return nil, nil, err
	}
	var spec corev1.PodSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
		return nil, nil, fmt.Errorf("decode Pod spec of %s %s: %w", object.GetKind(), object.GetName(), err)
	}
	return &spec, path, nil
}

// SetPodSpec writes a Pod spec back where PodSpecOf found it.
func SetPodSpec(object *unstructured.Unstructured, path []string, spec *corev1.PodSpec) error {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(spec)
	if err != nil {
		return fmt.Errorf("encode Pod spec of %s %s: %w", object.GetKind(), object.GetName(), err)
	}
	return unstructured.SetNestedMap(object.Object, raw, path...)
}
