// Package nodeagent keeps a cluster's node Agents as the Cluster's spec.nodeAgents declares them: one
// DaemonSet maintained by the cluster Agent, so a Kubernetes Node that joins and matches the selector gets
// its node Agent without anyone creating a workload for it.
package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/rollout"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	// Name names the DaemonSet, its ServiceAccount, ClusterRole and ClusterRoleBinding. It is the name
	// rlarkadm gives its node Agent DaemonSet, so both describe the same workload.
	Name = "rlark-agent-node"
	// HostRun is where a node Agent sees the host's /run. Every runtime socket kubeadm records on a Node
	// lies under /run (containerd, containerd-fleet, containerd-robot, k3s), so one DaemonSet serves all.
	HostRun = "/host/run"
	// CRISocketAnnotation is the runtime socket kubeadm records on each Node it joins.
	CRISocketAnnotation = "kubeadm.alpha.kubernetes.io/cri-socket"

	// ManagedByLabel marks the objects this controller owns. An object of the same name without it was
	// made by someone else (rlarkadm, an operator) and is never changed or deleted here; labelling it hands
	// it over.
	ManagedByLabel = "app.kubernetes.io/managed-by"
	ManagedBy      = "rlark-agent"
)

// Connection is how the cluster Agent itself reaches the Server. Its node Agents connect the same way,
// with the same client certificate, so they belong to the same RLark cluster.
type Connection struct {
	AgentType             string
	ServerAddress         string
	ServerHostname        string
	CACert                string
	ClientCert            string
	ClientKey             string
	InsecureSkipTLSVerify bool
	SSHAddress            string
	SSHHostKey            string
}

// Reconciler keeps the node Agent DaemonSet of one cluster.
type Reconciler struct {
	// Management reads the Cluster; the Server grants the cluster Agent read access to its own Cluster only.
	Management  client.Client
	ClusterName string
	// Local writes the DaemonSet and its RBAC in Namespace, the namespace the cluster Agent runs in.
	Local     client.Client
	Namespace string
	// PodName is the cluster Agent's own Pod: its certificate volume, host aliases and image pull secrets
	// are what the node Agents need to reach the Server as it does.
	PodName    string
	Connection Connection
	// LookupHost resolves a name as the cluster Agent does (net.LookupHost; tests replace it).
	LookupHost func(host string) ([]string, error)
}

// Setup watches the Cluster on the management side and the DaemonSet on the local side; either event
// reconciles the whole declaration.
func (r *Reconciler) Setup(managementManager, localManager ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(managementManager).
		Named("node-agents-cluster").
		For(&rlarkv1alpha1.Cluster{}, builder.WithPredicates(
			predicate.NewPredicateFuncs(func(o client.Object) bool { return o.GetName() == r.ClusterName }),
			// Status heartbeats do not change the declaration.
			predicate.GenerationChangedPredicate{},
		)).
		Complete(r); err != nil {
		return fmt.Errorf("setup node Agent Cluster watch: %w", err)
	}
	return ctrl.NewControllerManagedBy(localManager).
		Named("node-agents-daemonset").
		For(&appsv1.DaemonSet{}, builder.WithPredicates(
			predicate.NewPredicateFuncs(func(o client.Object) bool { return o.GetNamespace() == r.Namespace && o.GetName() == Name }),
			// Rollout status changes nothing to correct; spec edits and deletion do.
			predicate.GenerationChangedPredicate{},
		)).
		Complete(r)
}

// Reconcile makes the local objects match spec.nodeAgents. A Cluster that never declared node Agents
// leaves them alone, so existing hand-made node Agents keep running until the declaration is made.
func (r *Reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	var cluster rlarkv1alpha1.Cluster
	if err := r.Management.Get(ctx, client.ObjectKey{Name: r.ClusterName}, &cluster); err != nil {
		return reconcile.Result{}, fmt.Errorf("get Cluster %s: %w", r.ClusterName, err)
	}
	declared := cluster.Spec.NodeAgents
	if declared == nil {
		return reconcile.Result{}, nil
	}
	if !declared.Enabled {
		return reconcile.Result{}, r.remove(ctx)
	}
	if r.PodName == "" || r.Namespace == "" {
		return reconcile.Result{}, errors.New("node Agents need the cluster Agent's POD_NAME and POD_NAMESPACE from the downward API")
	}
	var self corev1.Pod
	if err := r.Local.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: r.PodName}, &self); err != nil {
		return reconcile.Result{}, fmt.Errorf("get the cluster Agent's own Pod: %w", err)
	}
	daemonSet, err := r.Render(declared, &self)
	if err != nil {
		return reconcile.Result{}, err
	}

	managed := labels.Set{ManagedByLabel: ManagedBy}
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: r.Namespace}}
	if err := r.apply(ctx, serviceAccount, func() { serviceAccount.Labels = labels.Merge(serviceAccount.Labels, managed) }); err != nil {
		return reconcile.Result{}, err
	}
	clusterRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: Name}}
	if err := r.apply(ctx, clusterRole, func() {
		clusterRole.Labels = labels.Merge(clusterRole.Labels, managed)
		clusterRole.Rules = []rbacv1.PolicyRule{
			// The node Agent reads its own Node (runtime socket), writes its machine's in-use
			// annotation on it and reports the Node's Kubernetes events.
			{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get", "patch"}},
			{APIGroups: []string{""}, Resources: []string{"events"}, Verbs: []string{"list", "watch"}},
		}
	}); err != nil {
		return reconcile.Result{}, err
	}
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: Name}}
	if err := r.apply(ctx, binding, func() {
		binding.Labels = labels.Merge(binding.Labels, managed)
		binding.Subjects = []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: Name, Namespace: r.Namespace}}
		binding.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: Name}
	}); err != nil {
		return reconcile.Result{}, err
	}
	existing := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: r.Namespace}}
	return reconcile.Result{}, r.apply(ctx, existing, func() {
		existing.Labels = labels.Merge(existing.Labels, daemonSet.Labels)
		existing.Annotations = labels.Merge(existing.Annotations, daemonSet.Annotations)
		// The API server fills in what the rendering leaves out, so the stored spec never equals the
		// rendered one. Replace it only when it lacks something rendered or keeps a selector key the
		// declaration dropped; an unchanged declaration then writes nothing.
		if !equality.Semantic.DeepDerivative(daemonSet.Spec, existing.Spec) ||
			!equality.Semantic.DeepEqual(daemonSet.Spec.Template.Spec.NodeSelector, existing.Spec.Template.Spec.NodeSelector) {
			existing.Spec = daemonSet.Spec
		}
	})
}

var errNotManaged = errors.New("exists and is not managed by the cluster Agent")

// apply creates obj or updates it with mutate, refusing an object someone else made.
func (r *Reconciler) apply(ctx context.Context, obj client.Object, mutate func()) error {
	result, err := controllerutil.CreateOrUpdate(ctx, r.Local, obj, func() error {
		if obj.GetResourceVersion() != "" && obj.GetLabels()[ManagedByLabel] != ManagedBy {
			return fmt.Errorf("%T %s %w; label it %s=%s to hand it over", obj, client.ObjectKeyFromObject(obj), errNotManaged, ManagedByLabel, ManagedBy)
		}
		mutate()
		return nil
	})
	if err != nil {
		return err
	}
	if result != controllerutil.OperationResultNone {
		log.FromContext(ctx).Info("node Agent object "+string(result), "kind", fmt.Sprintf("%T", obj), "name", obj.GetName())
	}
	return nil
}

// remove deletes the objects this controller owns. Objects someone else made stay.
func (r *Reconciler) remove(ctx context.Context) error {
	for _, obj := range []client.Object{
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: r.Namespace}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: Name}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: Name}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: r.Namespace}},
	} {
		if err := r.Local.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if obj.GetLabels()[ManagedByLabel] != ManagedBy {
			continue
		}
		if err := r.Local.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("delete %T %s: %w", obj, obj.GetName(), err)
		}
		log.FromContext(ctx).Info("node Agent object deleted", "kind", fmt.Sprintf("%T", obj), "name", obj.GetName())
	}
	return nil
}

// Render returns the node Agent DaemonSet for the declaration. self is the cluster Agent's own Pod.
func (r *Reconciler) Render(declared *rlarkv1alpha1.ClusterNodeAgents, self *corev1.Pod) (*appsv1.DaemonSet, error) {
	conn := r.Connection
	if declared.Image == "" {
		return nil, errors.New("the Cluster's spec.nodeAgents.image is not set: node Agents run the image declared for them")
	}

	// The certificate files are read from the volumes the cluster Agent reads them from.
	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	for _, path := range []string{conn.CACert, conn.ClientCert, conn.ClientKey} {
		if path == "" {
			continue
		}
		covered := false
		for _, container := range self.Spec.Containers {
			for _, mount := range container.VolumeMounts {
				if !strings.HasPrefix(path, strings.TrimSuffix(mount.MountPath, "/")+"/") {
					continue
				}
				covered = true
				if slices.ContainsFunc(mounts, func(m corev1.VolumeMount) bool { return m.Name == mount.Name }) {
					continue
				}
				mounts = append(mounts, corev1.VolumeMount{Name: mount.Name, MountPath: mount.MountPath, SubPath: mount.SubPath, ReadOnly: true})
				volumes = append(volumes, self.Spec.Volumes[slices.IndexFunc(self.Spec.Volumes, func(v corev1.Volume) bool { return v.Name == mount.Name })])
			}
		}
		if !covered {
			return nil, fmt.Errorf("the cluster Agent's %s is not on a volume of its Pod, so node Agents cannot mount it", path)
		}
	}
	directoryOrCreate := corev1.HostPathDirectoryOrCreate
	directory := corev1.HostPathDirectory
	hostToContainer := corev1.MountPropagationHostToContainer
	volumes = append(volumes,
		corev1.Volume{Name: "nodeserver-socket", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/rlark", Type: &directoryOrCreate}}},
		corev1.Volume{Name: "host-run", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/run", Type: &directory}}},
	)
	mounts = append(mounts,
		corev1.VolumeMount{Name: "nodeserver-socket", MountPath: "/var/run/rlark"},
		// Read-only is enough to connect to a Unix socket; HostToContainer follows a runtime that
		// recreates its socket directory after the Pod started.
		corev1.VolumeMount{Name: "host-run", MountPath: HostRun, ReadOnly: true, MountPropagation: &hostToContainer},
	)

	args := []string{
		"--mode=node",
		"--agent-type=" + conn.AgentType,
		"--server-address=" + conn.ServerAddress,
		"--leader-election=false",
		"--in-cluster",
		"--image-pull-enabled=true",
		"--rlark-server-ssh-address=" + conn.SSHAddress,
	}
	for _, flag := range [][2]string{{"--server-hostname", conn.ServerHostname}, {"--ca-cert", conn.CACert}, {"--client-cert", conn.ClientCert}, {"--client-key", conn.ClientKey}, {"--rlark-server-ssh-host-key", conn.SSHHostKey}} {
		if flag[1] != "" {
			args = append(args, flag[0]+"="+flag[1])
		}
	}
	if conn.InsecureSkipTLSVerify {
		args = append(args, "--insecure-skip-tls-verify")
	}

	// Node Agents reach the Server as the cluster Agent does. One in its node's network finds the Server by the
	// node's names (the site's, in /etc/hosts), which a pod does not see: the node Agents are given the addresses
	// it finds.
	hostAliases := self.Spec.HostAliases
	if self.Spec.HostNetwork {
		var err error
		if hostAliases, err = r.serverAliases(); err != nil {
			return nil, err
		}
	}

	fieldEnv := func(name, path string) corev1.EnvVar {
		return corev1.EnvVar{Name: name, ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: path}}}
	}
	selector := map[string]string{"app": Name}
	exists := corev1.TolerationOpExists
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name,
			Namespace: r.Namespace,
			Labels:    map[string]string{"app": Name, ManagedByLabel: ManagedBy},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec: corev1.PodSpec{
					ServiceAccountName: Name,
					NodeSelector:       declared.NodeSelector,
					// A node Agent is the Node's link to RLark: it must outlive ordinary workloads under
					// pressure and keep running on a Node that is cordoned, not ready or a control plane.
					PriorityClassName: "system-node-critical",
					Tolerations: []corev1.Toleration{
						{Key: "node-role.kubernetes.io/control-plane", Operator: exists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node.kubernetes.io/not-ready", Operator: exists, Effect: corev1.TaintEffectNoExecute},
						{Key: "node.kubernetes.io/unreachable", Operator: exists, Effect: corev1.TaintEffectNoExecute},
						{Key: "node.kubernetes.io/disk-pressure", Operator: exists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node.kubernetes.io/memory-pressure", Operator: exists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node.kubernetes.io/pid-pressure", Operator: exists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node.kubernetes.io/unschedulable", Operator: exists, Effect: corev1.TaintEffectNoSchedule},
						{Key: rlarkv1alpha1.LabelNodeCategory, Operator: exists},
					},
					HostAliases:      hostAliases,
					ImagePullSecrets: self.Spec.ImagePullSecrets,
					// The node server identifies a connecting workload by its host PID.
					HostPID: true,
					Containers: []corev1.Container{{
						Name:            Name,
						Image:           declared.Image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/usr/local/bin/agent"},
						Args:            args,
						Env: []corev1.EnvVar{
							fieldEnv("POD_NAME", "metadata.name"),
							fieldEnv("POD_NAMESPACE", "metadata.namespace"),
							fieldEnv("NODE_NAME", "spec.nodeName"),
							fieldEnv("POD_IP", "status.podIP"),
							{Name: "DISABLE_HTTP2", Value: "true"},
						},
						Ports: []corev1.ContainerPort{
							{Name: "metrics", ContainerPort: constants.AgentMetricsPort, Protocol: corev1.ProtocolTCP},
							{Name: "probes", ContainerPort: constants.AgentProbePort, Protocol: corev1.ProtocolTCP},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler:   corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/readyz", Port: intstr.FromInt32(constants.AgentProbePort)}},
							PeriodSeconds:  5,
							TimeoutSeconds: 3,
							// Spelled out although it is the API default: a defaulted integer would always
							// differ from an unset one, and every reconcile would rewrite the DaemonSet.
							SuccessThreshold: 1,
							FailureThreshold: 3,
						},
						SecurityContext: &corev1.SecurityContext{Privileged: utils.Ptr(true)},
						VolumeMounts:    mounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}
	// A node that is off does not hold the other nodes' node Agents back, and one node goes first.
	var policy rlarkv1alpha1.RolloutPolicy
	if declared.Rollout != nil {
		policy = *declared.Rollout
	}
	rollout.Roll(daemonSet, policy)
	return daemonSet, nil
}

// HostRunSocket is the node Agent's path to the runtime socket a Node records in CRISocketAnnotation.
func HostRunSocket(node *corev1.Node) (string, error) {
	recorded := node.Annotations[CRISocketAnnotation]
	if recorded == "" {
		return "", fmt.Errorf("node %s has no %s annotation; pass --containerd-socket", node.Name, CRISocketAnnotation)
	}
	path := strings.TrimPrefix(recorded, "unix://")
	for _, prefix := range []string{"/run/", "/var/run/"} {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			return HostRun + "/" + rest, nil
		}
	}
	return "", fmt.Errorf("node %s runtime socket %q is not under /run; pass --containerd-socket", node.Name, recorded)
}

// serverAliases are the Server's names in the cluster Agent's connection (its address and its SSH address) with the
// IPv4 address each resolves to here.
func (r *Reconciler) serverAliases() ([]corev1.HostAlias, error) {
	lookup := r.LookupHost
	if lookup == nil {
		lookup = net.LookupHost
	}
	var hosts []string
	if server, err := url.Parse(r.Connection.ServerAddress); err == nil && server.Hostname() != "" {
		hosts = append(hosts, server.Hostname())
	}
	if ssh := r.Connection.SSHAddress; ssh != "" {
		if host, _, err := net.SplitHostPort(ssh[strings.LastIndex(ssh, "@")+1:]); err == nil {
			hosts = append(hosts, host)
		}
	}
	var aliases []corev1.HostAlias
	for _, host := range hosts {
		if net.ParseIP(host) != nil || slices.ContainsFunc(aliases, func(a corev1.HostAlias) bool { return slices.Contains(a.Hostnames, host) }) {
			continue
		}
		addresses, err := lookup(host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s for the node Agents: %w", host, err)
		}
		i := slices.IndexFunc(addresses, func(a string) bool { return net.ParseIP(a).To4() != nil })
		if i < 0 {
			return nil, fmt.Errorf("%s resolves to no IPv4 address for the node Agents (%v)", host, addresses)
		}
		aliases = append(aliases, corev1.HostAlias{IP: addresses[i], Hostnames: []string{host}})
	}
	return aliases, nil
}
