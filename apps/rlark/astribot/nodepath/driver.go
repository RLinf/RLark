// Package nodepath is the CSI driver of RLark's host-path storages: a file
// system the nodes of a class have already mounted at one directory, such as
// Kingsoft Cloud KPFS, whose kernel client mounts it on the node over RDMA and
// which a Pod cannot mount again. The driver does not mount the file system;
// it bind-mounts the directory the node already has into the Pod, so reads and
// writes go to the node's own mount without FUSE or a copy.
//
// The storage's StorageClass carries the directory as parameter "root", the
// file system type it must be on as "fsType" and "readOnly" for the read
// profile. CreateVolume records them on the volume; NodePublishVolume checks
// that the root lies on a mounted file system of that type (not on the node's
// root file system, which a directory left behind by a failed mount would be
// on), bind-mounts it (the Pod's volumeMount subPath then selects a directory
// of it) and fails with the reason, which kubelet reports as the Pod's
// FailedMount event. DeleteVolume removes nothing: the data belongs to the
// storage, not to the claim.
//
// A bind does not follow a later remount of the node's file system: the
// Pod keeps the mount it got. The node plugin's Prober reports that, and
// which storages the node has (prober.go).
package nodepath

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Name is the driver's name, the provisioner of its StorageClasses.
	Name = "nodepath.csi.rlark.io"

	ParameterRoot     = "root"
	ParameterFSType   = "fsType"
	ParameterReadOnly = "readOnly"

	podNameKey      = "csi.storage.k8s.io/pod.name"
	podNamespaceKey = "csi.storage.k8s.io/pod.namespace"
)

// Mounter performs the node's mounts; tests replace it.
type Mounter interface {
	BindMount(source, target string, readOnly bool) error
	Unmount(target string) error
}

// Driver implements the CSI identity, controller and node services.
type Driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer

	NodeID  string
	Version string
	// HostRoot is where the node's file system is visible in the plugin's
	// container: a root /mnt/xc_kpfs/regular is HostRoot + that path.
	HostRoot string
	Mounter  Mounter
	Mounts   MountTable
	// Published records each target this node plugin bound.
	Published *PublishedStore
}

func (d *Driver) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{Name: Name, VendorVersion: d.Version}, nil
}

func (d *Driver) GetPluginCapabilities(context.Context, *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{Capabilities: []*csi.PluginCapability{{
		Type: &csi.PluginCapability_Service_{Service: &csi.PluginCapability_Service{Type: csi.PluginCapability_Service_CONTROLLER_SERVICE}},
	}}}, nil
}

func (d *Driver) Probe(context.Context, *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}

func (d *Driver) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: []*csi.ControllerServiceCapability{{
		Type: &csi.ControllerServiceCapability_Rpc{Rpc: &csi.ControllerServiceCapability_RPC{Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME}},
	}}}, nil
}

// cleanRoot validates a StorageClass's root directory.
func cleanRoot(root string) (string, error) {
	if !path.IsAbs(root) || path.Clean(root) != root || root == "/" {
		return "", status.Errorf(codes.InvalidArgument, "parameter %s must be a clean absolute directory other than /, got %q", ParameterRoot, root)
	}
	return root, nil
}

// CreateVolume names a volume of the root: the claim's PersistentVolume.
func (d *Driver) CreateVolume(_ context.Context, request *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if request.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name is required")
	}
	root, err := cleanRoot(request.GetParameters()[ParameterRoot])
	if err != nil {
		return nil, err
	}
	readOnly := request.GetParameters()[ParameterReadOnly]
	if readOnly != "" && readOnly != "true" && readOnly != "false" {
		return nil, status.Errorf(codes.InvalidArgument, "parameter %s must be true or false", ParameterReadOnly)
	}
	for key := range request.GetParameters() {
		if key != ParameterRoot && key != ParameterReadOnly && key != ParameterFSType && !strings.HasPrefix(key, "csi.storage.k8s.io/") {
			return nil, status.Errorf(codes.InvalidArgument, "unknown parameter %q", key)
		}
	}
	return &csi.CreateVolumeResponse{Volume: &csi.Volume{
		VolumeId:      request.GetName(),
		CapacityBytes: request.GetCapacityRange().GetRequiredBytes(),
		VolumeContext: map[string]string{ParameterRoot: root, ParameterReadOnly: readOnly, ParameterFSType: request.GetParameters()[ParameterFSType]},
	}}, nil
}

// DeleteVolume leaves the directory: it is the storage's.
func (d *Driver) DeleteVolume(_ context.Context, request *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if request.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ValidateVolumeCapabilities(_ context.Context, request *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, capability := range request.GetVolumeCapabilities() {
		if capability.GetBlock() != nil {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: "block volumes are not supported"}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: request.GetVolumeCapabilities()}}, nil
}

func (d *Driver) NodeGetInfo(context.Context, *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{NodeId: d.NodeID}, nil
}

func (d *Driver) NodeGetCapabilities(context.Context, *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{}, nil
}

// NodePublishVolume bind-mounts the node's directory at the Pod's target.
// Publishing again an already published target succeeds.
func (d *Driver) NodePublishVolume(_ context.Context, request *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	target := request.GetTargetPath()
	if target == "" || request.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id and target path are required")
	}
	if request.GetVolumeCapability().GetBlock() != nil {
		return nil, status.Error(codes.InvalidArgument, "block volumes are not supported")
	}
	root, err := cleanRoot(request.GetVolumeContext()[ParameterRoot])
	if err != nil {
		return nil, err
	}
	mounts, err := d.Mounts()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read mount table: %v", err)
	}
	if mountedAt(mounts, target) {
		return &csi.NodePublishVolumeResponse{}, nil
	}
	fsType := request.GetVolumeContext()[ParameterFSType]
	mount, err := CheckRoot(mounts, d.HostRoot, d.NodeID, root, fsType)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error()+": the host-path storage is not mounted on this node")
	}
	readOnly := request.GetReadonly() || request.GetVolumeContext()[ParameterReadOnly] == "true"
	if err := os.MkdirAll(target, 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "create target %s: %v", target, err)
	}
	if err := d.Mounter.BindMount(filepath.Join(d.HostRoot, root), target, readOnly); err != nil {
		return nil, status.Errorf(codes.Internal, "bind %s at %s: %v", root, target, err)
	}
	if err := d.Published.Put(Published{Target: target, Root: root, FSType: fsType, Device: mount.Device,
		PodNamespace: request.GetVolumeContext()[podNamespaceKey], PodName: request.GetVolumeContext()[podNameKey]}); err != nil {
		return nil, status.Errorf(codes.Internal, "record %s: %v", target, err)
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the target; an absent target is unpublished.
func (d *Driver) NodeUnpublishVolume(_ context.Context, request *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	target := request.GetTargetPath()
	if target == "" || request.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id and target path are required")
	}
	mounts, err := d.Mounts()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read mount table: %v", err)
	}
	if mountedAt(mounts, target) {
		if err := d.Mounter.Unmount(target); err != nil {
			return nil, status.Errorf(codes.Internal, "unmount %s: %v", target, err)
		}
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "remove target %s: %v", target, err)
	}
	if err := d.Published.Delete(target); err != nil {
		return nil, status.Errorf(codes.Internal, "forget %s: %v", target, err)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// LinuxMounter mounts with the kernel's bind mounts.
type LinuxMounter struct{}

func (LinuxMounter) BindMount(source, target string, readOnly bool) error {
	if err := unix.Mount(source, target, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return err
	}
	if !readOnly {
		return nil
	}
	// A bind mount takes read-only only on a remount of itself.
	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_RDONLY, ""); err != nil {
		_ = unix.Unmount(target, 0)
		return fmt.Errorf("remount read-only: %w", err)
	}
	return nil
}

func (LinuxMounter) Unmount(target string) error { return unix.Unmount(target, 0) }

// mountedAt reports whether something is mounted exactly at target.
func mountedAt(mounts []Mount, target string) bool {
	for _, mount := range mounts {
		if mount.MountPoint == target {
			return true
		}
	}
	return false
}
