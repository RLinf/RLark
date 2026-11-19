package nodeagent

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const namespace = "rlark-system"

func scheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// clusterAgentPod is the cluster Agent's own Pod as rlarkadm renders it, plus the host alias a site adds.
func clusterAgentPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "rlark-agent-0", Namespace: namespace},
		Spec: corev1.PodSpec{
			HostAliases:      []corev1.HostAlias{{IP: "10.11.14.128", Hostnames: []string{"rlark-server"}}},
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry"}},
			Containers: []corev1.Container{{
				Name: "rlark-agent",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "addon-catalog", MountPath: "/etc/rlark/addons"},
					{Name: "cert", MountPath: "/etc/rlark/certs", ReadOnly: true},
				},
			}},
			Volumes: []corev1.Volume{
				{Name: "addon-catalog", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "rlark-addon-catalog"}}}},
				{Name: "cert", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "rlark-agent-cert"}}},
			},
		},
	}
}

func cluster(nodeAgents *rlarkv1alpha1.ClusterNodeAgents) *rlarkv1alpha1.Cluster {
	return &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "rlark-farm"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-farm", NodeAgents: nodeAgents},
	}
}

func reconciler(t *testing.T, management *rlarkv1alpha1.Cluster, local ...client.Object) *Reconciler {
	s := scheme(t)
	return &Reconciler{
		Management:  fake.NewClientBuilder().WithScheme(s).WithObjects(management).Build(),
		ClusterName: management.Name,
		Local:       fake.NewClientBuilder().WithScheme(s).WithObjects(append(local, clusterAgentPod())...).Build(),
		Namespace:   namespace,
		PodName:     "rlark-agent-0",
		Connection: Connection{
			AgentType:     "Kubernetes",
			ServerAddress: "https://rlark-server:8443",
			CACert:        "/etc/rlark/certs/ca.crt",
			ClientCert:    "/etc/rlark/certs/tls.crt",
			ClientKey:     "/etc/rlark/certs/tls.key",
			SSHAddress:    "client@rlark-server:2222",
		},
	}
}

func enabled() *rlarkv1alpha1.ClusterNodeAgents {
	return &rlarkv1alpha1.ClusterNodeAgents{Enabled: true, Image: "registry.local/rlark:v1", NodeSelector: map[string]string{"rlark.io/enabled": "true"}}
}

func TestRenderFollowsTheClusterAgent(t *testing.T) {
	r := reconciler(t, cluster(enabled()))
	ds, err := r.Render(enabled(), clusterAgentPod())
	if err != nil {
		t.Fatal(err)
	}
	pod := ds.Spec.Template.Spec
	container := pod.Containers[0]
	if ds.Name != Name || ds.Namespace != namespace || ds.Labels[ManagedByLabel] != ManagedBy {
		t.Fatalf("metadata = %+v", ds.ObjectMeta)
	}
	if container.Image != "registry.local/rlark:v1" || pod.ServiceAccountName != Name || !pod.HostPID || pod.PriorityClassName != "system-node-critical" {
		t.Fatalf("pod = %+v", pod)
	}
	for _, arg := range []string{
		"--mode=node", "--image-pull-enabled=true", "--server-address=https://rlark-server:8443", "--rlark-server-ssh-address=client@rlark-server:2222",
		"--ca-cert=/etc/rlark/certs/ca.crt", "--client-cert=/etc/rlark/certs/tls.crt", "--client-key=/etc/rlark/certs/tls.key",
	} {
		if !slices.Contains(container.Args, arg) {
			t.Errorf("args %v lack %s", container.Args, arg)
		}
	}
	if slices.ContainsFunc(container.Args, func(a string) bool { return strings.HasPrefix(a, "--containerd-socket") }) {
		t.Errorf("args %v must leave the socket to the Node's annotation", container.Args)
	}
	if pod.NodeSelector["rlark.io/enabled"] != "true" || len(pod.NodeSelector) != 1 {
		t.Errorf("nodeSelector = %v", pod.NodeSelector)
	}
	if pod.HostAliases[0].IP != "10.11.14.128" || pod.ImagePullSecrets[0].Name != "registry" {
		t.Errorf("host aliases %v / pull secrets %v not taken from the cluster Agent", pod.HostAliases, pod.ImagePullSecrets)
	}
	var volumes []string
	for _, v := range pod.Volumes {
		volumes = append(volumes, v.Name)
	}
	if !slices.Equal(volumes, []string{"cert", "nodeserver-socket", "host-run"}) {
		t.Errorf("volumes = %v, want only the certificate volume of the cluster Agent plus the host paths", volumes)
	}
	if pod.Volumes[0].Secret.SecretName != "rlark-agent-cert" || pod.Volumes[2].HostPath.Path != "/run" || *pod.Volumes[2].HostPath.Type != corev1.HostPathDirectory {
		t.Errorf("volumes = %+v", pod.Volumes)
	}
	hostRun := container.VolumeMounts[2]
	if hostRun.MountPath != HostRun || !hostRun.ReadOnly {
		t.Errorf("host run mount = %+v", hostRun)
	}
	if container.ReadinessProbe.HTTPGet.Path != "/readyz" || container.ReadinessProbe.HTTPGet.Port.IntValue() != 10382 {
		t.Errorf("readiness = %+v", container.ReadinessProbe)
	}
	var keys []string
	for _, toleration := range pod.Tolerations {
		if toleration.Key == "" {
			t.Fatal("a blanket toleration would also keep the node Agent on an out-of-service Node")
		}
		if toleration.TolerationSeconds != nil {
			t.Errorf("toleration %s must not expire", toleration.Key)
		}
		keys = append(keys, toleration.Key)
	}
	for _, key := range []string{"node-role.kubernetes.io/control-plane", "node.kubernetes.io/not-ready", "node.kubernetes.io/unreachable", "node.kubernetes.io/unschedulable", "node.kubernetes.io/disk-pressure", rlarkv1alpha1.LabelNodeCategory} {
		if !slices.Contains(keys, key) {
			t.Errorf("tolerations %v lack %s", keys, key)
		}
	}
	if slices.Contains(keys, "node.kubernetes.io/out-of-service") {
		t.Error("an out-of-service Node must lose its node Agent")
	}
}

// The node Agents are rolled by RLark, as the Cluster declares or by the defaults: a node that is off does not
// hold the others back.
func TestRenderIsRolledByRLark(t *testing.T) {
	r := reconciler(t, cluster(enabled()))
	ds, err := r.Render(enabled(), clusterAgentPod())
	if err != nil {
		t.Fatal(err)
	}
	if ds.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType || ds.Annotations[rlarkv1alpha1.RolloutAnnotation] != `{}` {
		t.Fatalf("strategy=%#v annotations=%v", ds.Spec.UpdateStrategy, ds.Annotations)
	}
	declared := enabled()
	canary, bound := int32(2), intstr.FromString("25%")
	declared.Rollout = &rlarkv1alpha1.RolloutPolicy{Canary: &canary, MaxUnavailable: &bound}
	if ds, err = r.Render(declared, clusterAgentPod()); err != nil || ds.Annotations[rlarkv1alpha1.RolloutAnnotation] != `{"canary":2,"maxUnavailable":"25%"}` {
		t.Fatalf("annotations=%v err=%v", ds.Annotations, err)
	}
}

// The image of the tools the cluster Agent gives Task Pods (its --image) is no part of a node Agent: the cluster
// Agent started with another one renders the same DaemonSet.
func TestRenderHasNothingOfTheToolsImage(t *testing.T) {
	ds, err := reconciler(t, cluster(enabled())).Render(enabled(), clusterAgentPod())
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range ds.Spec.Template.Spec.Containers[0].Args {
		if strings.HasPrefix(arg, "--image=") {
			t.Fatalf("the node Agent is given %s", arg)
		}
	}
}

func TestRenderRefusesWhatNodeAgentsCannotUse(t *testing.T) {
	r := reconciler(t, cluster(enabled()))
	undeclared := enabled()
	undeclared.Image = ""
	if _, err := r.Render(undeclared, clusterAgentPod()); err == nil {
		t.Error("node Agents whose image is not declared must not be rendered")
	}
	r = reconciler(t, cluster(enabled()))
	r.Connection.ClientCert = "/var/lib/agent/tls.crt"
	if _, err := r.Render(enabled(), clusterAgentPod()); err == nil {
		t.Error("a certificate the node Agents cannot mount must be refused")
	}
}

func TestReconcileCreatesUpdatesAndRemoves(t *testing.T) {
	ctx := context.Background()
	r := reconciler(t, cluster(enabled()))
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	var ds appsv1.DaemonSet
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: namespace, Name: Name}, &ds); err != nil {
		t.Fatal(err)
	}
	for _, obj := range []client.Object{
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: Name}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: Name}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: Name}},
	} {
		if err := r.Local.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil || obj.GetLabels()[ManagedByLabel] != ManagedBy {
			t.Fatalf("%T: %v %v", obj, err, obj.GetLabels())
		}
	}

	// Reconciling an unchanged declaration again writes nothing, even after the API server filled in
	// defaults the rendering leaves out.
	ds.Spec.RevisionHistoryLimit = new(int32(10))
	ds.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
	ds.Spec.Template.Spec.Containers[0].ReadinessProbe.SuccessThreshold = 1
	ds.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTP
	if err := r.Local.Update(ctx, &ds); err != nil {
		t.Fatal(err)
	}
	defaulted := ds.ResourceVersion
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: namespace, Name: Name}, &ds); err != nil {
		t.Fatal(err)
	}
	if ds.ResourceVersion != defaulted {
		t.Fatalf("an unchanged declaration rewrote the DaemonSet (resourceVersion %s -> %s)", defaulted, ds.ResourceVersion)
	}

	// The node Agents' image is the one declared for them: declaring another rolls them.
	var another rlarkv1alpha1.Cluster
	if err := r.Management.Get(ctx, client.ObjectKey{Name: r.ClusterName}, &another); err != nil {
		t.Fatal(err)
	}
	another.Spec.NodeAgents.Image = "registry.local/rlark:v2"
	if err := r.Management.Update(ctx, &another); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: namespace, Name: Name}, &ds); err != nil {
		t.Fatal(err)
	}
	if ds.Spec.Template.Spec.Containers[0].Image != "registry.local/rlark:v2" {
		t.Fatalf("image = %s", ds.Spec.Template.Spec.Containers[0].Image)
	}

	// A changed selector follows too.
	declared := cluster(&rlarkv1alpha1.ClusterNodeAgents{Enabled: true, Image: "registry.local/rlark:v2", NodeSelector: map[string]string{"rlark.io/node-category": "robot"}})
	r.Management = fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(declared).Build()
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: namespace, Name: Name}, &ds); err != nil {
		t.Fatal(err)
	}
	if len(ds.Spec.Template.Spec.NodeSelector) != 1 || ds.Spec.Template.Spec.NodeSelector["rlark.io/node-category"] != "robot" {
		t.Fatalf("nodeSelector = %v, want only the declared key", ds.Spec.Template.Spec.NodeSelector)
	}

	// Disabled: everything the controller made goes.
	r.Management = fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(cluster(&rlarkv1alpha1.ClusterNodeAgents{Enabled: false})).Build()
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	for _, obj := range []client.Object{
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: Name}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: Name}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: Name}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: Name}},
	} {
		if err := r.Local.Get(ctx, client.ObjectKeyFromObject(obj), obj); !apierrors.IsNotFound(err) {
			t.Errorf("%T still there after disabling: %v", obj, err)
		}
	}
}

func TestReconcileLeavesOthersObjectsAlone(t *testing.T) {
	ctx := context.Background()
	foreign := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: Name, Labels: map[string]string{"app": Name}}}

	// No declaration: the cluster Agent does not manage node Agents at all.
	r := reconciler(t, cluster(nil), foreign.DeepCopy())
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	var sa corev1.ServiceAccount
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: namespace, Name: Name}, &sa); !apierrors.IsNotFound(err) {
		t.Fatalf("nothing may be created without a declaration: %v", err)
	}

	// Enabled next to a DaemonSet someone else made (rlarkadm): refused, not adopted.
	r = reconciler(t, cluster(enabled()), foreign.DeepCopy())
	if _, err := r.Reconcile(ctx, reconcile.Request{}); !errors.Is(err, errNotManaged) {
		t.Fatalf("err = %v, want errNotManaged", err)
	}
	// Disabled: that DaemonSet stays.
	r = reconciler(t, cluster(&rlarkv1alpha1.ClusterNodeAgents{Enabled: false}), foreign.DeepCopy())
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	var ds appsv1.DaemonSet
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: namespace, Name: Name}, &ds); err != nil {
		t.Fatalf("a DaemonSet the cluster Agent did not make must stay: %v", err)
	}

	// Labelled by the operator: handed over, and taken into the declared form.
	handed := foreign.DeepCopy()
	handed.Labels[ManagedByLabel] = ManagedBy
	r = reconciler(t, cluster(enabled()), handed)
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: namespace, Name: Name}, &ds); err != nil || ds.Spec.Template.Spec.Containers[0].Image != "registry.local/rlark:v1" {
		t.Fatalf("handed-over DaemonSet not reconciled: %v %+v", err, ds.Spec.Template.Spec)
	}
}

func TestHostRunSocket(t *testing.T) {
	for _, tt := range []struct {
		annotation, want string
	}{
		{"unix:///run/containerd-fleet/containerd.sock", "/host/run/containerd-fleet/containerd.sock"},
		{"unix:///run/containerd-robot/containerd.sock", "/host/run/containerd-robot/containerd.sock"},
		{"/run/containerd/containerd.sock", "/host/run/containerd/containerd.sock"},
		{"unix:///var/run/containerd/containerd.sock", "/host/run/containerd/containerd.sock"},
		{"unix:///opt/runtime/containerd.sock", ""},
		{"", ""},
	} {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "s1u-02-x86"}}
		if tt.annotation != "" {
			node.Annotations = map[string]string{CRISocketAnnotation: tt.annotation}
		}
		got, err := HostRunSocket(node)
		if got != tt.want || (err != nil) != (tt.want == "") {
			t.Errorf("HostRunSocket(%q) = %q, %v; want %q", tt.annotation, got, err, tt.want)
		}
	}
}

// A cluster Agent in its node's network finds the Server by the node's names; node Agents, in the pod network, are
// given the address it finds (11-17: a new cluster's node agents could not resolve rlark-server).
func TestRenderGivesNodeAgentsTheAddressAHostNetworkAgentFinds(t *testing.T) {
	r := reconciler(t, cluster(enabled()))
	r.LookupHost = func(host string) ([]string, error) {
		if host != "rlark-server" {
			t.Fatalf("looked up %s", host)
		}
		return []string{"fe80::1", "198.18.99.1"}, nil
	}
	self := clusterAgentPod()
	self.Spec.HostNetwork = true
	self.Spec.HostAliases = nil
	ds, err := r.Render(enabled(), self)
	if err != nil {
		t.Fatal(err)
	}
	aliases := ds.Spec.Template.Spec.HostAliases
	if len(aliases) != 1 || aliases[0].IP != "198.18.99.1" || !slices.Equal(aliases[0].Hostnames, []string{"rlark-server"}) {
		t.Fatalf("host aliases %+v", aliases)
	}
	r.LookupHost = func(string) ([]string, error) { return nil, errors.New("no such host") }
	if _, err := r.Render(enabled(), self); err == nil || !strings.Contains(err.Error(), "rlark-server") {
		t.Fatalf("err = %v, want the name that does not resolve", err)
	}
}
