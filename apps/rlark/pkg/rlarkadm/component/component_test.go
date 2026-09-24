package component

import (
	"slices"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestKCPWorkloadStrategy(t *testing.T) {
	tests := []struct {
		name          string
		etcd          *types.EtcdConfig
		kcpReplicas   int32
		wantKind      string
		wantReplicas  int32
		wantClaim     bool
		wantDataMount bool
	}{
		{
			name:          "embedded storage uses single replica statefulset",
			kcpReplicas:   1,
			wantKind:      "StatefulSet",
			wantReplicas:  1,
			wantClaim:     true,
			wantDataMount: true,
		},
		{
			name:         "deployed etcd uses single replica deployment",
			etcd:         &types.EtcdConfig{},
			kcpReplicas:  1,
			wantKind:     "",
			wantReplicas: 1,
		},
		{
			name:         "external etcd uses single replica deployment",
			etcd:         &types.EtcdConfig{Address: "https://etcd.example.com:2379"},
			kcpReplicas:  1,
			wantKind:     "",
			wantReplicas: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &types.DeployConfig{
				Plane: types.PlaneControl,
				Kubernetes: &types.KubernetesEnv{
					Etcd: tt.etcd,
					KCP: &types.ComponentConfig{
						Replicas: tt.kcpReplicas,
						Storage: &types.StorageConfig{
							Type: types.StoragePVC,
							Size: "20Gi",
						},
					},
				},
			}

			var kcp *types.Component
			for _, c := range ComponentsForPlane(cfg) {
				if c.Name == constants.ComponentKCP {
					component := c
					kcp = &component
					break
				}
			}
			if kcp == nil {
				t.Fatal("kcp component not found")
			}
			if kcp.WorkloadKind != tt.wantKind {
				t.Fatalf("WorkloadKind = %q, want %q", kcp.WorkloadKind, tt.wantKind)
			}

			if tt.wantKind == "StatefulSet" {
				sts := StatefulSet(cfg, kcp)
				if got := *sts.Spec.Replicas; got != tt.wantReplicas {
					t.Fatalf("replicas = %d, want %d", got, tt.wantReplicas)
				}
				if got := len(sts.Spec.VolumeClaimTemplates) > 0; got != tt.wantClaim {
					t.Fatalf("has volume claim = %v, want %v", got, tt.wantClaim)
				}
				hasDataMount := len(sts.Spec.Template.Spec.Containers[0].VolumeMounts) > 0
				if hasDataMount != tt.wantDataMount {
					t.Fatalf("has data mount = %v, want %v", hasDataMount, tt.wantDataMount)
				}
				if hasDataMount {
					mount := sts.Spec.Template.Spec.Containers[0].VolumeMounts[0]
					if mount.Name != "kcp-data" || mount.MountPath != constants.KCPDataDir {
						t.Fatalf("data mount = %s:%s, want kcp-data:%s", mount.Name, mount.MountPath, constants.KCPDataDir)
					}
				}
				return
			}

			dep := Deployment(cfg, kcp)
			if got := *dep.Spec.Replicas; got != tt.wantReplicas {
				t.Fatalf("replicas = %d, want %d", got, tt.wantReplicas)
			}
			if len(dep.Spec.Template.Spec.Volumes) != 0 {
				t.Fatalf("deployment has %d data volumes, want none", len(dep.Spec.Template.Spec.Volumes))
			}
		})
	}
}

func TestGlobalReplicasDoNotScaleKCP(t *testing.T) {
	cfg := &types.DeployConfig{
		Plane: types.PlaneControl,
		Kubernetes: &types.KubernetesEnv{
			Replicas: 3,
			Etcd:     &types.EtcdConfig{Replicas: 3},
		},
	}
	for _, c := range ComponentsForPlane(cfg) {
		if c.Name == constants.ComponentKCP {
			if got := *Deployment(cfg, &c).Spec.Replicas; got != 1 {
				t.Fatalf("kcp replicas = %d, want 1", got)
			}
			return
		}
	}
	t.Fatal("kcp component not found")
}

func TestKubernetesManagementAPIComponents(t *testing.T) {
	cfg := &types.DeployConfig{
		Plane: types.PlaneControl,
		Kubernetes: &types.KubernetesEnv{
			ManagementAPI:          "kubernetes",
			GatewayImage:           "gateway:test",
			ControllerManagerImage: "controller:test",
			ServerImage:            "server:test",
		},
	}

	components := ComponentsForPlane(cfg)
	for _, c := range components {
		if c.Name == constants.ComponentKCP || c.Name == constants.ComponentEtcd {
			t.Fatalf("unexpected component %q in kubernetes management API mode", c.Name)
		}
		switch c.Name {
		case constants.ComponentGateway, constants.ComponentControllerManager, constants.ComponentServer:
			dep := Deployment(cfg, &c)
			container := dep.Spec.Template.Spec.Containers[0]
			if !slices.Contains(container.Args, "--in-cluster") {
				t.Fatalf("%s args do not contain --in-cluster: %#v", c.Name, container.Args)
			}
			if slices.Contains(container.Args, "--kubeconfig") {
				t.Fatalf("%s still uses --kubeconfig: %#v", c.Name, container.Args)
			}
			if !slices.Contains(container.Args, "--kube-namespace="+constants.Namespace) {
				t.Fatalf("%s does not use deployment namespace: %#v", c.Name, container.Args)
			}
			if len(container.VolumeMounts) != 0 {
				t.Fatalf("%s has unexpected kubeconfig mounts: %#v", c.Name, container.VolumeMounts)
			}
			if dep.Spec.Template.Spec.ServiceAccountName != c.Name {
				t.Fatalf("%s service account = %q, want %q", c.Name, dep.Spec.Template.Spec.ServiceAccountName, c.Name)
			}
			if sa, role, binding := RBAC(cfg, &c); sa == nil || role == nil || binding == nil {
				t.Fatalf("%s management API RBAC is incomplete", c.Name)
			}
		}
	}
}

func TestManagementAPIRBACAllowsEventRecording(t *testing.T) {
	cfg := &types.DeployConfig{Kubernetes: &types.KubernetesEnv{ManagementAPI: "kubernetes"}}
	for _, rule := range managementAPIRBAC(cfg) {
		if slices.Contains(rule.APIGroups, "") && slices.Contains(rule.Resources, "events") {
			if !slices.Contains(rule.Verbs, "*") {
				t.Fatalf("events rule verbs = %v, want all verbs", rule.Verbs)
			}
			return
		}
	}
	t.Fatal("events RBAC rule not found")
}

func TestManagementAPIRBACAllowsControlPlaneResources(t *testing.T) {
	cfg := &types.DeployConfig{Kubernetes: &types.KubernetesEnv{ManagementAPI: "kubernetes"}}
	want := map[string][]string{
		"":                          {"configmaps", "events", "namespaces", "secrets", "serviceaccounts"},
		"rbac.authorization.k8s.io": {"roles", "rolebindings", "clusterroles", "clusterrolebindings"},
		"coordination.k8s.io":       {"leases"},
	}
	for group, resources := range want {
		for _, resource := range resources {
			found := false
			for _, rule := range managementAPIRBAC(cfg) {
				if slices.Contains(rule.APIGroups, group) && slices.Contains(rule.Resources, resource) && slices.Contains(rule.Verbs, "*") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing all-verbs RBAC rule for %s/%s", group, resource)
			}
		}
	}
}

func TestNodeAgentUsesDedicatedRBAC(t *testing.T) {
	cfg := &types.DeployConfig{
		Plane:      types.PlaneData,
		Kubernetes: &types.KubernetesEnv{},
	}

	for _, c := range ComponentsForPlane(cfg) {
		if c.Name != constants.ComponentAgentNode {
			continue
		}

		ds := DaemonSet(cfg, &c)
		if got := ds.Spec.Template.Spec.ServiceAccountName; got != constants.ComponentAgentNode {
			t.Fatalf("node-agent service account = %q, want %q", got, constants.ComponentAgentNode)
		}
		container := ds.Spec.Template.Spec.Containers[0]
		if container.LivenessProbe != nil {
			t.Fatalf("node-agent liveness probe = %#v, want nil", container.LivenessProbe)
		}
		if container.ReadinessProbe == nil || container.ReadinessProbe.HTTPGet == nil || container.ReadinessProbe.HTTPGet.Path != "/readyz" {
			t.Fatalf("node-agent readiness probe = %#v", container.ReadinessProbe)
		}

		sa, role, binding := RBAC(cfg, &c)
		if sa == nil || role == nil || binding == nil {
			t.Fatal("node-agent RBAC is incomplete")
		}
		if sa.Name != constants.ComponentAgentNode || role.Name != constants.ComponentAgentNode || binding.Name != constants.ComponentAgentNode {
			t.Fatalf("node-agent RBAC names = %q, %q, %q, want %q", sa.Name, role.Name, binding.Name, constants.ComponentAgentNode)
		}

		want := map[string][]string{
			"nodes":  {"get"},
			"events": {"list", "watch"},
		}
		if len(role.Rules) != len(want) {
			t.Fatalf("node-agent RBAC rules = %#v, want %d rules", role.Rules, len(want))
		}
		for resource, verbs := range want {
			found := false
			for _, rule := range role.Rules {
				if slices.Equal(rule.APIGroups, []string{""}) && slices.Equal(rule.Resources, []string{resource}) && slices.Equal(rule.Verbs, verbs) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing node-agent RBAC rule for %s with verbs %v", resource, verbs)
			}
		}
		return
	}

	t.Fatal("node-agent component not found")
}

func TestKubernetesWorkloadsUseImagePullSecrets(t *testing.T) {
	cfg := &types.DeployConfig{
		Kubernetes: &types.KubernetesEnv{
			ImagePullSecrets: []string{"registry-one", "registry-two"},
		},
	}
	c := &types.Component{
		Name:    "test",
		ImageFn: func(*types.DeployConfig) string { return "test:latest" },
		ArgsFn:  func(*types.DeployConfig) []string { return nil },
	}

	for name, refs := range map[string][]corev1.LocalObjectReference{
		"deployment":  Deployment(cfg, c).Spec.Template.Spec.ImagePullSecrets,
		"daemonset":   DaemonSet(cfg, c).Spec.Template.Spec.ImagePullSecrets,
		"statefulset": StatefulSet(cfg, c).Spec.Template.Spec.ImagePullSecrets,
	} {
		names := make([]string, 0, len(refs))
		for _, ref := range refs {
			names = append(names, ref.Name)
		}
		if !slices.Equal(names, cfg.Kubernetes.ImagePullSecrets) {
			t.Errorf("%s imagePullSecrets = %v, want %v", name, names, cfg.Kubernetes.ImagePullSecrets)
		}
	}
}

func TestControllerManagerLeaderElectionArgs(t *testing.T) {
	tests := []struct {
		name          string
		managementAPI string
		wantNamespace string
	}{
		{name: "kcp", wantNamespace: "default"},
		{name: "kubernetes", managementAPI: "kubernetes", wantNamespace: constants.Namespace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &types.DeployConfig{
				Plane: types.PlaneControl,
				Kubernetes: &types.KubernetesEnv{
					ManagementAPI:          tt.managementAPI,
					ControllerManagerImage: "controller:test",
					Replicas:               2,
				},
			}
			for _, c := range ComponentsForPlane(cfg) {
				if c.Name != constants.ComponentControllerManager {
					continue
				}
				args := Deployment(cfg, &c).Spec.Template.Spec.Containers[0].Args
				if !slices.Contains(args, "--leader-election=true") {
					t.Fatalf("args do not enable leader election: %#v", args)
				}
				if !slices.Contains(args, "--kube-namespace="+tt.wantNamespace) {
					t.Fatalf("args do not set leader election namespace %q: %#v", tt.wantNamespace, args)
				}
				return
			}
			t.Fatal("controller manager component not found")
		})
	}
}

func TestEtcdStatefulSetStorageAndStableIdentity(t *testing.T) {
	cfg := &types.DeployConfig{
		Plane: types.PlaneControl,
		Kubernetes: &types.KubernetesEnv{
			EtcdImage: "etcd:test",
			Etcd: &types.EtcdConfig{
				Replicas: 3,
				Storage: &types.StorageConfig{
					Type: types.StoragePVC,
					Size: "8Gi",
				},
			},
		},
	}

	var etcd *types.Component
	for _, c := range ComponentsForPlane(cfg) {
		if c.Name == constants.ComponentEtcd {
			component := c
			etcd = &component
			break
		}
	}
	if etcd == nil {
		t.Fatal("etcd component not found")
	}

	sts := StatefulSet(cfg, etcd)
	if sts.Spec.ServiceName != constants.ComponentEtcd {
		t.Fatalf("serviceName = %q, want %q", sts.Spec.ServiceName, constants.ComponentEtcd)
	}
	if sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement {
		t.Fatalf("podManagementPolicy = %q, want %q", sts.Spec.PodManagementPolicy, appsv1.ParallelPodManagement)
	}
	if len(sts.Spec.VolumeClaimTemplates) != 1 || sts.Spec.VolumeClaimTemplates[0].Name != "etcd-data" {
		t.Fatalf("volumeClaimTemplates = %#v, want etcd-data claim", sts.Spec.VolumeClaimTemplates)
	}
	mounts := sts.Spec.Template.Spec.Containers[0].VolumeMounts
	if len(mounts) != 1 || mounts[0].Name != "etcd-data" || mounts[0].MountPath != constants.EtcdDataDir {
		t.Fatalf("volume mounts = %#v, want etcd-data:%s", mounts, constants.EtcdDataDir)
	}

	args := sts.Spec.Template.Spec.Containers[0].Args
	if !slices.Contains(args, "--initial-advertise-peer-urls=http://$(POD_NAME).etcd.rlark-system.svc:2380") {
		t.Fatalf("args do not advertise stable peer DNS: %#v", args)
	}
	if !slices.Contains(args, "--initial-cluster=etcd-0=http://etcd-0.etcd.rlark-system.svc:2380,etcd-1=http://etcd-1.etcd.rlark-system.svc:2380,etcd-2=http://etcd-2.etcd.rlark-system.svc:2380") {
		t.Fatalf("args do not contain expected initial cluster: %#v", args)
	}

	svc := Service(cfg, etcd)
	if svc.Spec.ClusterIP != "None" || !svc.Spec.PublishNotReadyAddresses {
		t.Fatalf("etcd service is not headless with publishNotReadyAddresses: %#v", svc.Spec)
	}
}

func TestPostgresqlStorage(t *testing.T) {
	tests := []struct {
		name             string
		globalStorage    *types.StorageConfig
		componentStorage *types.StorageConfig
		wantEmptyDir     bool
		wantHostPath     string
		wantPVC          bool
		wantSize         resource.Quantity
		wantStorageClass string
		wantNodeSelector map[string]string
	}{
		{
			name:         "default uses emptyDir",
			wantEmptyDir: true,
		},
		{
			name: "global hostPath",
			globalStorage: &types.StorageConfig{
				Type:         types.StorageHostPath,
				HostPath:     "/data/postgresql",
				NodeSelector: map[string]string{"storage": "local"},
			},
			wantHostPath:     "/data/postgresql",
			wantNodeSelector: map[string]string{"storage": "local"},
		},
		{
			name: "global pvc",
			globalStorage: &types.StorageConfig{
				Type:         types.StoragePVC,
				StorageClass: "fast",
				Size:         "40Gi",
			},
			wantPVC:          true,
			wantSize:         resource.MustParse("40Gi"),
			wantStorageClass: "fast",
		},
		{
			name: "component pvc overrides global storage",
			globalStorage: &types.StorageConfig{
				Type:     types.StorageHostPath,
				HostPath: "/global",
			},
			componentStorage: &types.StorageConfig{
				Type:         types.StoragePVC,
				NodeSelector: map[string]string{"database": "true"},
			},
			wantPVC:          true,
			wantSize:         resource.MustParse("30Gi"),
			wantNodeSelector: map[string]string{"database": "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &types.DeployConfig{
				Plane: types.PlaneControl,
				DB:    &types.DBConfig{},
				Kubernetes: &types.KubernetesEnv{
					Storage: tt.globalStorage,
					Postgresql: &types.ComponentConfig{
						Storage: tt.componentStorage,
					},
				},
			}

			var postgresql *types.Component
			for _, c := range ComponentsForPlane(cfg) {
				if c.Name == constants.ComponentPostgresql {
					component := c
					postgresql = &component
					break
				}
			}
			if postgresql == nil {
				t.Fatal("postgresql component not found")
			}

			dep := Deployment(cfg, postgresql)
			if !mapsEqual(dep.Spec.Template.Spec.NodeSelector, tt.wantNodeSelector) {
				t.Fatalf("node selector = %#v, want %#v", dep.Spec.Template.Spec.NodeSelector, tt.wantNodeSelector)
			}
			dataVolume, found := volumeByName(dep.Spec.Template.Spec.Volumes, "pg-data")
			if !found {
				t.Fatal("pg-data volume not found")
			}
			if got := dataVolume.EmptyDir != nil; got != tt.wantEmptyDir {
				t.Fatalf("pg-data emptyDir = %v, want %v", got, tt.wantEmptyDir)
			}
			if tt.wantHostPath != "" {
				if dataVolume.HostPath == nil || dataVolume.HostPath.Path != tt.wantHostPath || dataVolume.HostPath.Type == nil || *dataVolume.HostPath.Type != corev1.HostPathDirectoryOrCreate {
					t.Fatalf("pg-data hostPath = %#v, want %q with DirectoryOrCreate", dataVolume.HostPath, tt.wantHostPath)
				}
			}
			if got := dataVolume.PersistentVolumeClaim != nil; got != tt.wantPVC {
				t.Fatalf("pg-data uses PVC = %v, want %v", got, tt.wantPVC)
			}

			claims := postgresql.VolumeClaimFn(cfg)
			if got := len(claims) == 1; got != tt.wantPVC {
				t.Fatalf("has PVC = %v, want %v", got, tt.wantPVC)
			}
			if tt.wantPVC {
				claim := claims[0]
				if claim.Name != "pg-data" || claim.Spec.Resources.Requests.Storage().Cmp(tt.wantSize) != 0 {
					t.Fatalf("claim = %#v, want pg-data with size %s", claim, tt.wantSize.String())
				}
				if tt.wantStorageClass == "" {
					if claim.Spec.StorageClassName != nil {
						t.Fatalf("storage class = %q, want cluster default", *claim.Spec.StorageClassName)
					}
				} else if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName != tt.wantStorageClass {
					t.Fatalf("storage class = %#v, want %q", claim.Spec.StorageClassName, tt.wantStorageClass)
				}
			}

			mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
			if !slices.ContainsFunc(mounts, func(m corev1.VolumeMount) bool {
				return m.Name == "pg-data" && m.MountPath == constants.PostgresqlDataDir
			}) {
				t.Fatalf("postgresql data mount not found: %#v", mounts)
			}
			if !slices.ContainsFunc(mounts, func(m corev1.VolumeMount) bool {
				return m.Name == "pg-init" && m.ReadOnly
			}) {
				t.Fatalf("postgresql init mount not found: %#v", mounts)
			}
		})
	}
}

func volumeByName(volumes []corev1.Volume, name string) (corev1.Volume, bool) {
	for _, volume := range volumes {
		if volume.Name == name {
			return volume, true
		}
	}
	return corev1.Volume{}, false
}

func mapsEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}
