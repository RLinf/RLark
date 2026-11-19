package gateway

// Rendering a storage declaration into what the distribution controllers
// apply. A storage has one primary Replication declaration, which holds the
// stored declaration and reaches every cluster that mounts it through the
// storage's default endpoint, and one endpoint declaration per cluster that
// mounts it through an endpoint of its own. Each declaration delivers, to each
// of its clusters:
//
//   - the storage's View (ConfigMap <id>-storage in the cluster's namespace of
//     the management plane), which Job and Addon volumes are resolved against;
//   - a Delivery of one StorageClass per mount profile to the cluster itself:
//     rclone's, with its Secret, for object storage and SMB; the node
//     directory driver's (apps/rlark/astribot/nodepath) for a host-path storage.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/rlinf/rlark/apps/rlark/astribot/nodepath"
	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// storageClassEndpointLabel marks an endpoint declaration; its value is the
// storage's id.
const storageClassEndpointLabel = "rlark.io/storageclass-endpoint"

func storageEndpointSecretName(id, cluster string) string { return id + "-at-" + cluster }

// storageProfileClass is one StorageClass of a storage: the one a volume gets
// when it chooses the profile.
type storageProfileClass struct {
	Profile      string
	Name         string
	Secret       string
	MountOptions []string
	Backend      map[string]string
}

// storageOwnProfile is the profile of a volume that names none.
func storageOwnProfile(stored storedStorageClass) string {
	if stored.Type == storageTypeS3 && stored.MountProfile != "" {
		return stored.MountProfile
	}
	return storageref.ProfileDefault
}

// storageProfileClasses lists a storage's StorageClasses. The storage's own
// profile keeps the class sc-<id> it always had; every other profile is
// sc-<id>-<profile>; the own profile comes first. A profile the storage's mount options cannot be combined
// with (a record from before profiles were chosen per volume) is not offered.
func storageProfileClasses(id string, stored storedStorageClass) ([]storageProfileClass, error) {
	own := storageOwnProfile(stored)
	profiles := []string{own}
	for _, profile := range storageref.Profiles(stored.Type) {
		if profile != own {
			profiles = append(profiles, profile)
		}
	}
	var classes []storageProfileClass
	for _, profile := range profiles {
		class := storageProfileClass{Profile: profile, Name: id + "-" + profile, Secret: id + "-" + profile + "-secret"}
		if profile == own {
			class.Name, class.Secret = id, id+"-secret"
		}
		switch stored.Type {
		case storageTypeHostPath:
			// The node plugin binds the directory read-only for the read profile.
		case storageTypeSMB:
			class.MountOptions = slices.Clone(smbMountOptions)
			if profile == storageref.ProfileRead {
				class.MountOptions = append(class.MountOptions, "read-only=true")
			}
		default:
			cacheMode := ""
			if profile == own {
				cacheMode = stored.VFSCacheMode
			}
			options, backend, _, err := resolveS3MountOptions(profile, cacheMode, stored.MountOptions)
			if err != nil {
				if profile == own {
					return nil, err
				}
				continue
			}
			class.MountOptions, class.Backend = options, backend
		}
		classes = append(classes, class)
	}
	return classes, nil
}

// storageView is what every cluster of the storage resolves volumes against.
func storageView(id string, stored storedStorageClass) (storageref.View, error) {
	view := storageref.View{Name: stored.Name, Type: stored.Type, DefaultProfile: storageOwnProfile(stored)}
	if stored.Type == storageTypeHostPath {
		// The nodes the driver's node plugin found to have it.
		view.NodeSelector = map[string]string{nodepath.NodeLabel(id): "true"}
	}
	classes, err := storageProfileClasses(id, stored)
	if err != nil {
		return storageref.View{}, err
	}
	view.StorageClasses = make(map[string]string, len(classes))
	for _, class := range classes {
		view.StorageClasses[class.Profile] = class.Name
	}
	return view, nil
}

// storageMountEndpoint is the endpoint the cluster's rclone mounts use.
func storageMountEndpoint(stored storedStorageClass, cluster string) string {
	if endpoint := stored.ClusterMountEndpoints[cluster]; endpoint != "" {
		return endpoint
	}
	if stored.MountEndpoint != "" {
		return stored.MountEndpoint
	}
	return stored.Endpoint
}

// storageDelivery renders the Delivery of a storage's StorageClasses to one
// cluster, with the rclone remote reaching the storage through mountEndpoint.
func storageDelivery(id string, stored storedStorageClass, mountEndpoint string) (v1.Secret, error) {
	classes, err := storageProfileClasses(id, stored)
	if err != nil {
		return v1.Secret{}, err
	}
	remote, remotePath := "s3", stored.Bucket
	if stored.Prefix != "" {
		remotePath += "/" + stored.Prefix
	}
	var base strings.Builder
	switch stored.Type {
	case storageTypeHostPath:
		return storageHostPathDelivery(id, stored, classes)
	case storageTypeSMB:
		// rclone's smb backend only accepts an obscured password.
		pass, err := storage.RcloneObscure(stored.Password)
		if err != nil {
			return v1.Secret{}, fmt.Errorf("obscure smb password: %w", err)
		}
		data := struct{ Host, User, Pass, Domain string }{stored.Host, stored.Username, pass, stored.Domain}
		if err := smbSecretTmpl.Execute(&base, data); err != nil {
			return v1.Secret{}, fmt.Errorf("render secret config: %w", err)
		}
		remote, remotePath = "smb", smbRemotePath(stored)
	default:
		// The remote is "s3" for every class: a StorageClass's parameters are
		// immutable, and the node plugin (astribot/csi-rclone-ks3) gives each
		// published volume its own remote, so classes never share one.
		request := CreateStorageClassRequest{
			Provider: stored.Provider, Endpoint: mountEndpoint, AccessKeyId: stored.AccessKeyID, AccessKeySecret: stored.AccessKey,
			Region: stored.Region, PathStyle: stored.PathStyle,
		}
		if err := ssSecretTmpl.Execute(&base, request); err != nil {
			return v1.Secret{}, fmt.Errorf("render secret config: %w", err)
		}
	}
	annotations := map[string]string{annoStorageClassDescription: stored.Description}
	if problems := validation.IsValidLabelValue(stored.Name); len(problems) == 0 {
		annotations[annoStorageClassName] = stored.Name
	}
	manifests := make([]any, 0, 2*len(classes))
	for _, class := range classes {
		config := base.String()
		for _, key := range slices.Sorted(maps.Keys(class.Backend)) {
			config += fmt.Sprintf("%s = %s\n", key, class.Backend[key])
		}
		classAnnotations := maps.Clone(annotations)
		classAnnotations[annoStorageProfile] = class.Profile
		manifests = append(manifests,
			v1.Secret{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
				ObjectMeta: metav1.ObjectMeta{Name: class.Secret, Namespace: metav1.NamespaceSystem, Labels: map[string]string{"controller": "rlark"}},
				StringData: map[string]string{"remote": remote, "remotePath": remotePath, "configData": config},
			},
			storagev1.StorageClass{
				TypeMeta:   metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"},
				ObjectMeta: metav1.ObjectMeta{Name: class.Name, Labels: map[string]string{"controller": "rlark"}, Annotations: classAnnotations},
				// Delete removes the PersistentVolume object with its claim; the driver's
				// DeleteVolume does nothing to the remote. Retain left one Released
				// volume per Task per submission.
				Provisioner: "rclone.csi.veloxpack.io", ReclaimPolicy: ptr.To(v1.PersistentVolumeReclaimDelete),
				VolumeBindingMode: ptr.To(storagev1.VolumeBindingImmediate), AllowVolumeExpansion: ptr.To(true),
				Parameters: map[string]string{
					"remote": remote, "remotePath": remotePath,
					"csi.storage.k8s.io/node-publish-secret-name":      class.Secret,
					"csi.storage.k8s.io/node-publish-secret-namespace": metav1.NamespaceSystem,
				},
				MountOptions: slices.Clone(class.MountOptions),
			})
	}
	return storageDeliverySecret(id, manifests)
}

// storageHostPathDelivery renders a host-path storage's StorageClasses: the
// node directory CSI driver (apps/rlark/astribot/nodepath) binds the storage's directory,
// and a claim binds once its Pod is scheduled, on a node that has it.
func storageHostPathDelivery(id string, stored storedStorageClass, classes []storageProfileClass) (v1.Secret, error) {
	manifests := make([]any, 0, len(classes))
	for _, class := range classes {
		annotations := map[string]string{annoStorageClassDescription: stored.Description, annoStorageProfile: class.Profile}
		if problems := validation.IsValidLabelValue(stored.Name); len(problems) == 0 {
			annotations[annoStorageClassName] = stored.Name
		}
		parameters := map[string]string{nodepath.ParameterRoot: stored.Path}
		if stored.FSType != "" {
			parameters[nodepath.ParameterFSType] = stored.FSType
		}
		if class.Profile == storageref.ProfileRead {
			parameters[nodepath.ParameterReadOnly] = "true"
		}
		manifests = append(manifests, storagev1.StorageClass{
			TypeMeta:   metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"},
			ObjectMeta: metav1.ObjectMeta{Name: class.Name, Labels: map[string]string{"controller": "rlark", nodepath.StorageIDLabel: id}, Annotations: annotations},
			// Delete removes the PersistentVolume with its claim; the driver's
			// DeleteVolume leaves the directory.
			Provisioner: nodepath.Name, ReclaimPolicy: ptr.To(v1.PersistentVolumeReclaimDelete),
			VolumeBindingMode: ptr.To(storagev1.VolumeBindingWaitForFirstConsumer),
			Parameters:        parameters,
		})
	}
	return storageDeliverySecret(id, manifests)
}

// storageDeliverySecret wraps a storage's manifests for one cluster.
func storageDeliverySecret(id string, manifests []any) (v1.Secret, error) {
	storageManifests, err := yaml.Marshal(manifests)
	if err != nil {
		return v1.Secret{}, fmt.Errorf("marshal storage class manifests: %w", err)
	}
	deliveryConfig := distribution.Config{Apply: distribution.ApplyPolicy{Mode: "ServerSideApply", ConflictPolicy: "Fail", DeletionPolicy: "Delete"}}
	deliveryConfig.Security.AllowClusterScoped = true
	deliveryConfigYAML, err := yaml.Marshal(deliveryConfig)
	if err != nil {
		return v1.Secret{}, fmt.Errorf("marshal delivery config: %w", err)
	}
	return v1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   storageClassDeliveryName(id),
			Labels: map[string]string{storageClassDeliveryLabel: "true"},
		},
		Type: distribution.DeliverySecretType,
		Data: map[string][]byte{"config": deliveryConfigYAML, "manifests": storageManifests},
	}, nil
}

// annoStorageProfile names the mount profile of a delivered StorageClass.
const annoStorageProfile = "rlark.io/storage-profile"

func storageViewConfigMap(id string, view storageref.View) (v1.ConfigMap, error) {
	data, err := json.Marshal(view)
	if err != nil {
		return v1.ConfigMap{}, fmt.Errorf("marshal storage view: %w", err)
	}
	return v1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: storageref.ViewConfigMapName(id), Labels: map[string]string{storageref.ViewLabel: "true"}},
		Data:       map[string]string{storageref.ViewKey: string(data)},
	}, nil
}

// storageReplication renders one Replication declaration delivering the
// storage to clusters through mountEndpoint.
func storageReplication(namespace, name string, labels map[string]string, id string, stored storedStorageClass, clusters []string, mountEndpoint string) (*v1.Secret, error) {
	view, err := storageView(id, stored)
	if err != nil {
		return nil, err
	}
	viewConfigMap, err := storageViewConfigMap(id, view)
	if err != nil {
		return nil, err
	}
	// The Delivery comes first: the distribution engine applies in order.
	delivery, err := storageDelivery(id, stored, mountEndpoint)
	if err != nil {
		return nil, err
	}
	objects := []any{delivery, viewConfigMap}
	manifest, err := yaml.Marshal(objects)
	if err != nil {
		return nil, fmt.Errorf("marshal replication manifests: %w", err)
	}
	config := distribution.Config{Apply: distribution.ApplyPolicy{Mode: "ServerSideApply", ConflictPolicy: "Fail", DeletionPolicy: "Delete"}}
	config.Targets.Namespaces = &distribution.GlobSelector{Include: append([]string{}, clusters...), Exclude: []string{"rlark-system"}}
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal replication config: %w", err)
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Type:       distribution.ReplicationSecretType,
		Data:       map[string][]byte{"config": configYAML, "manifests": manifest},
	}, nil
}

// buildStorageClassReplicationSecret renders the primary declaration: the
// stored declaration, delivered to the clusters that use the default endpoint.
func buildStorageClassReplicationSecret(namespace, id string, stored storedStorageClass) (*v1.Secret, error) {
	clusters := make([]string, 0, len(stored.Clusters))
	for _, cluster := range stored.Clusters {
		if _, own := stored.ClusterMountEndpoints[cluster]; !own {
			clusters = append(clusters, cluster)
		}
	}
	secret, err := storageReplication(namespace, storageClassReplicationName(id), map[string]string{storageClassReplicationLabel: "true"}, id, stored, clusters, storageMountEndpoint(stored, ""))
	if err != nil {
		return nil, err
	}
	storedJSON, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("marshal storage class: %w", err)
	}
	secret.Data["storageclass"] = storedJSON
	return secret, nil
}

// buildStorageEndpointSecrets renders one declaration per cluster that mounts
// the storage through an endpoint of its own, sorted by cluster.
func buildStorageEndpointSecrets(namespace, id string, stored storedStorageClass) ([]*v1.Secret, error) {
	clusters := slices.Sorted(maps.Keys(stored.ClusterMountEndpoints))
	secrets := make([]*v1.Secret, 0, len(clusters))
	for _, cluster := range clusters {
		secret, err := storageReplication(namespace, storageEndpointSecretName(id, cluster), map[string]string{storageClassEndpointLabel: id}, id, stored, []string{cluster}, storageMountEndpoint(stored, cluster))
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}

// smbPasswordLine matches the obscured SMB password in a rendered rclone
// config. rclone obscures with a random IV, so the line differs on every render.
var smbPasswordLine = regexp.MustCompile(`(?m)^(\s*pass = ).*$`)

// storageClassRenderKey reduces a Replication declaration to what its render
// means, so that two renders of one stored declaration compare equal.
func storageClassRenderKey(data map[string][]byte) string {
	manifests, err := yaml.YAMLToJSON(data["manifests"])
	if err != nil {
		return ""
	}
	var objects []json.RawMessage
	if err := json.Unmarshal(manifests, &objects); err != nil {
		return ""
	}
	parts := []string{string(data["storageclass"]), string(data["config"])}
	for _, object := range objects {
		var delivery v1.Secret
		if err := json.Unmarshal(object, &delivery); err == nil && delivery.Type == distribution.DeliverySecretType {
			parts = append(parts, string(delivery.Data["config"]), string(smbPasswordLine.ReplaceAll(delivery.Data["manifests"], []byte("${1}*"))))
			continue
		}
		parts = append(parts, string(object))
	}
	return strings.Join(parts, "\x00")
}

// syncStorageEndpointSecrets makes the storage's endpoint declarations the
// rendered ones: it creates and updates them and deletes those of clusters
// that no longer have an endpoint of their own (or of a deleted storage, when
// stored is nil). A changed SMB password render alone is not a change.
func (g *Gateway) syncStorageEndpointSecrets(ctx context.Context, id string, stored *storedStorageClass, identity *v1.Secret) error {
	namespace := g.managementNamespace()
	existing, err := g.rawClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{storageClassEndpointLabel: id}.AsSelector().String(),
	})
	if err != nil {
		return fmt.Errorf("list storage endpoint declarations: %w", err)
	}
	current := make(map[string]*v1.Secret, len(existing.Items))
	for i := range existing.Items {
		current[existing.Items[i].Name] = &existing.Items[i]
	}
	var desired []*v1.Secret
	if stored != nil {
		if desired, err = buildStorageEndpointSecrets(namespace, id, *stored); err != nil {
			return err
		}
	}
	for _, secret := range desired {
		old, found := current[secret.Name]
		delete(current, secret.Name)
		if !found {
			tenant.SetIdentity(secret, tenant.IDOf(identity), identity.Annotations[tenant.CreatedByAnnotation])
			if _, err := g.rawClient.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create storage endpoint declaration %s: %w", secret.Name, err)
			}
			continue
		}
		if storageClassRenderKey(old.Data) == storageClassRenderKey(secret.Data) {
			continue
		}
		old.Data = secret.Data
		if _, err := g.rawClient.CoreV1().Secrets(namespace).Update(ctx, old, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update storage endpoint declaration %s: %w", old.Name, err)
		}
	}
	for name := range current {
		if err := g.rawClient.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("delete storage endpoint declaration %s: %w", name, err)
		}
	}
	return nil
}
