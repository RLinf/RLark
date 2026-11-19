package gateway

import (
	"context"
	"slices"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// storageTypeS3 is the default for records written before types existed.
	storageTypeS3       = "s3"
	storageTypeSMB      = "smb"
	storageTypeHostPath = storageref.TypeHostPath
)

// s3MountOptions and smbMountOptions are rclone VFS flags for the CSI mount.
// fast-list only applies to bucket-based remotes, so SMB omits it.
var (
	s3MountOptions  = []string{"vfs-cache-mode=writes", "dir-cache-time=5m", "attr-timeout=1m", "poll-interval=5s", "vfs-write-back=1s", "vfs-cache-max-age=10m", "buffer-size=64M", "transfers=8", "checkers=16", "fast-list"}
	smbMountOptions = []string{"vfs-cache-mode=writes", "dir-cache-time=5m", "attr-timeout=1m", "poll-interval=5s", "vfs-write-back=1s", "vfs-cache-max-age=10m", "buffer-size=64M", "transfers=8", "checkers=8"}
)

func effectiveVFSCacheMode(mode string) string {
	if mode == "" {
		return "writes"
	}
	return mode
}

func s3MountOptionsForCacheMode(mode string) []string {
	options := slices.Clone(s3MountOptions)
	if effectiveVFSCacheMode(mode) == "off" {
		for i, option := range options {
			if strings.HasPrefix(option, "vfs-cache-mode=") {
				options[i] = "vfs-cache-mode=off"
				break
			}
		}
	}
	return options
}

// rerenderStorageClasses rewrites every storage declaration whose rendered
// objects are not what this Gateway renders from its stored fields. A storage
// is otherwise rendered only when it is created or edited, so a change in the
// render (reclaim policy, mount options of a profile) would reach existing
// storages one manual edit at a time. One storage failing does not stop the rest.
func (g *Gateway) rerenderStorageClasses(ctx context.Context) {
	logger := log.FromContext(ctx)
	secrets, err := listStorageClassSecrets(ctx, g.rawClient, g.managementNamespace())
	if err != nil {
		logger.Error(err, "list storage class replications to re-render")
		return
	}
	for i := range secrets {
		secret := &secrets[i]
		id, stored, err := storageClassFromSecret(secret)
		if err != nil {
			logger.Error(err, "skip re-rendering storage class", "secret", secret.Name)
			continue
		}
		rebuilt, err := buildStorageClassReplicationSecret(secret.Namespace, id, stored)
		if err != nil {
			logger.Error(err, "re-render storage class", "storage", stored.Name)
			continue
		}
		if err := g.syncStorageEndpointSecrets(ctx, id, &stored, secret); err != nil {
			logger.Error(err, "re-render storage endpoint declarations", "storage", stored.Name)
		}
		if storageClassRenderKey(secret.Data) == storageClassRenderKey(rebuilt.Data) {
			continue
		}
		secret.Data = rebuilt.Data
		if _, err := g.rawClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			logger.Error(err, "update re-rendered storage class", "storage", stored.Name)
			continue
		}
		logger.Info("re-rendered storage class", "storage", stored.Name, "storageClass", id)
	}
}
