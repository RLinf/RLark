package addon

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

func TestCleanupAddonOnlyDeletesCurrentInstanceAndWaits(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "current"}}
	makeConfig := func(name, uid string) *corev1.ConfigMap {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "rlark-system", UID: types.UID(name + "-uid"), Labels: map[string]string{"rlark.io/addon-name": "a", "rlark.io/addon-uid": uid}}}
	}
	cl := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(makeConfig("owned", "current"), makeConfig("other", "old"), makeConfig("legacy", "")).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	if err := r.cleanupAddon(ctx, addon); !errors.Is(err, errAddonCleanupPending) {
		t.Fatalf("first cleanup = %v, want pending", err)
	}
	if err := r.cleanupAddon(ctx, addon); err != nil {
		t.Fatalf("second cleanup = %v", err)
	}
	for _, name := range []string{"other", "legacy"} {
		var got corev1.ConfigMap
		if err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "rlark-system"}, &got); err != nil {
			t.Fatalf("unowned resource %s removed: %v", name, err)
		}
	}
	addon.UID = ""
	if err := r.cleanupAddon(ctx, addon); err == nil {
		t.Fatal("accepted missing addon UID")
	}
}

type failOnceUpdateClient struct {
	client.Client
	fail bool
}

type failOnceDeleteClient struct {
	client.Client
	fail bool
}

type relabelBeforeDeleteClient struct {
	client.Client
	once bool
}

func (c *relabelBeforeDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if !c.once {
		c.once = true
		var current corev1.ConfigMap
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), &current); err != nil {
			return err
		}
		current.Labels["rlark.io/addon-uid"] = "replacement"
		if err := c.Update(ctx, &current); err != nil {
			return err
		}
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestCleanupDoesNotDeleteRelabelledResource(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "uid"}}
	resource := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "rlark-system", UID: "object-uid", Labels: map[string]string{"rlark.io/addon-name": "a", "rlark.io/addon-uid": "uid"}}}
	cl := &relabelBeforeDeleteClient{Client: fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(resource).Build()}
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	if err := r.cleanupAddon(ctx, addon); err == nil {
		t.Fatal("stale delete succeeded after ownership changed")
	}
	var got corev1.ConfigMap
	if err := cl.Get(ctx, client.ObjectKeyFromObject(resource), &got); err != nil {
		t.Fatalf("relabelled resource was deleted: %v", err)
	}
	if got.Labels["rlark.io/addon-uid"] != "replacement" {
		t.Fatal("replacement ownership lost")
	}
	if err := r.cleanupAddon(ctx, addon); err != nil {
		t.Fatalf("retry after relabel: %v", err)
	}
}

func (c *failOnceDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.fail {
		c.fail = false
		return errors.New("temporary delete failure")
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestCleanupAddonRetriesDeleteFailure(t *testing.T) {
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "uid"}}
	resource := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "rlark-system", UID: "object-uid", Labels: map[string]string{"rlark.io/addon-name": "a", "rlark.io/addon-uid": "uid"}}}
	cl := &failOnceDeleteClient{Client: fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(resource).Build(), fail: true}
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	if err := r.cleanupAddon(context.Background(), addon); err == nil || errors.Is(err, errAddonCleanupPending) {
		t.Fatalf("delete failure was not returned: %v", err)
	}
	if err := r.cleanupAddon(context.Background(), addon); !errors.Is(err, errAddonCleanupPending) {
		t.Fatalf("retry = %v, want pending", err)
	}
	if err := r.cleanupAddon(context.Background(), addon); err != nil {
		t.Fatalf("cleanup after deletion = %v", err)
	}
}

func (c *failOnceUpdateClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.fail {
		c.fail = false
		return errors.New("temporary update failure")
	}
	return c.Client.Update(ctx, obj, opts...)
}

func TestApplyObjectRetriesAfterUpdateFailure(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "uid", ResourceVersion: "1"}}
	makeConfig := func(value string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{"name": "test", "namespace": "rlark-system", "labels": map[string]interface{}{"rlark.io/addon-name": "a", "rlark.io/addon-uid": "uid"}, "annotations": map[string]interface{}{AddonGenerationAnnotation: "1"}},
			"data":     map[string]interface{}{"key": value},
		}}
	}
	cl := &failOnceUpdateClient{Client: fake.NewClientBuilder().WithObjects(makeConfig("old")).Build(), fail: true}
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	if err := r.applyObject(ctx, makeConfig("new"), addon); err == nil {
		t.Fatal("update failure was ignored")
	}
	if err := r.applyObject(ctx, makeConfig("new"), addon); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	got := makeConfig("old")
	if err := cl.Get(ctx, types.NamespacedName{Name: "test", Namespace: "rlark-system"}, got); err != nil {
		t.Fatal(err)
	}
	if value, _, _ := unstructured.NestedString(got.Object, "data", "key"); value != "new" {
		t.Fatalf("retry left value %q", value)
	}
}

func TestApplyObjectRepairsOwnedDriftAndRejectsForeignObjects(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "csi", UID: types.UID("addon-uid"), Generation: 42}}
	makeObject := func(uid string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "driver", "namespace": "rlark-system", "labels": map[string]interface{}{"rlark.io/addon-name": "csi", "rlark.io/addon-uid": uid}, "annotations": map[string]interface{}{AddonGenerationAnnotation: "42"}},
			"spec":     map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"nodeSelector": map[string]interface{}{"kubernetes.io/hostname": "worker"}}}},
		}}
	}
	for _, tt := range []struct {
		name, uid string
		wantErr   bool
	}{{"owned", "addon-uid", false}, {"different instance", "other", true}, {"unowned", "", true}} {
		t.Run(tt.name, func(t *testing.T) {
			live := makeObject(tt.uid)
			unstructured.RemoveNestedField(live.Object, "spec", "template", "spec", "nodeSelector")
			cl := fake.NewClientBuilder().WithObjects(live).Build()
			r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
			err := r.applyObject(ctx, makeObject("addon-uid"), addon)
			if (err != nil) != tt.wantErr {
				t.Fatalf("apply: %v", err)
			}
			got := makeObject(tt.uid)
			if err := cl.Get(ctx, types.NamespacedName{Name: "driver", Namespace: "rlark-system"}, got); err != nil {
				t.Fatal(err)
			}
			_, found, err := unstructured.NestedMap(got.Object, "spec", "template", "spec", "nodeSelector")
			if err != nil || found == tt.wantErr {
				t.Fatalf("selector found=%v err=%v", found, err)
			}
			if !tt.wantErr {
				version := got.GetResourceVersion()
				if err := r.applyObject(ctx, makeObject("addon-uid"), addon); err != nil {
					t.Fatal(err)
				}
				if err := cl.Get(ctx, types.NamespacedName{Name: "driver", Namespace: "rlark-system"}, got); err != nil {
					t.Fatal(err)
				}
				if got.GetResourceVersion() != version {
					t.Fatal("unchanged object updated again")
				}
			}
		})
	}
}

func TestDesiredFieldsMatchAllowsDefaultedFields(t *testing.T) {
	desired := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{"replicas": int64(1)}}}
	existing := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{"replicas": int64(1), "defaulted": "value"}}}
	if !desiredFieldsMatch(desired, existing) {
		t.Fatal("server-defaulted field considered drift")
	}
}

func TestDesiredFieldsMatchDetectsZeroValuesAndListDrift(t *testing.T) {
	for _, pair := range [][2]interface{}{{false, true}, {int64(0), int64(1)}, {"", "nonempty"}, {[]interface{}{"one"}, []interface{}{"one", "two"}}} {
		if desiredValueMatches(pair[0], pair[1]) {
			t.Errorf("accepted drift: wanted %#v, got %#v", pair[0], pair[1])
		}
	}
}

func TestSecretStringDataComparedAsStoredData(t *testing.T) {
	desired := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "stringData": map[string]interface{}{"key": "value"}}}
	existing := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "data": map[string]interface{}{"key": "dmFsdWU="}}}
	if !desiredFieldsMatch(desired, existing) {
		t.Fatal("equivalent Secret treated as drift")
	}
	existing.Object["data"] = map[string]interface{}{"key": "b3RoZXI="}
	if desiredFieldsMatch(desired, existing) {
		t.Fatal("Secret drift ignored")
	}
	if _, ok := desired.Object["stringData"]; !ok {
		t.Fatal("comparison mutated desired object")
	}
}

func TestReadyAddonSchedulesDriftReconciliation(t *testing.T) {
	ctx := context.Background()
	a := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "csi", Namespace: "test", UID: "uid", Finalizers: []string{AddonFinalizer}}, Spec: rlarkv1alpha1.AddonSpec{AddonName: "csi-driver-rclone", Version: "v1"}}
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	mgmt := fake.NewClientBuilder().WithScheme(scheme).WithObjects(a, publishedBuiltIn(t, "csi-driver-rclone", "v1", "test")).WithStatusSubresource(&rlarkv1alpha1.Addon{}).Build()
	replicas := int32(1)
	local := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&appsv1.Deployment{}, &appsv1.DaemonSet{}).WithObjects(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: "rlark-system", Labels: map[string]string{"rlark.io/addon-name": "csi"}}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ready"}}}, Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1}}).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{ManagementClient: mgmt, LocalKubeClient: local, AgentType: string(rlarkv1alpha1.AgentTypeKubernetes)}}}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "csi", Namespace: "test"}}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	var dep appsv1.Deployment
	if err := local.Get(ctx, types.NamespacedName{Name: "csi-driver-rclone-controller", Namespace: "rlark-system"}, &dep); err != nil {
		t.Fatal(err)
	}
	dep.Status.ObservedGeneration = dep.Generation
	dep.Status.Replicas, dep.Status.UpdatedReplicas, dep.Status.ReadyReplicas = *dep.Spec.Replicas, *dep.Spec.Replicas, *dep.Spec.Replicas
	if err := local.Status().Update(ctx, &dep); err != nil {
		t.Fatal(err)
	}
	var ds appsv1.DaemonSet
	if err := local.Get(ctx, types.NamespacedName{Name: "csi-driver-rclone-node", Namespace: "rlark-system"}, &ds); err != nil {
		t.Fatal(err)
	}
	ds.Status.ObservedGeneration = ds.Generation
	ds.Status.DesiredNumberScheduled, ds.Status.UpdatedNumberScheduled, ds.Status.NumberReady = 1, 1, 1
	if err := local.Status().Update(ctx, &ds); err != nil {
		t.Fatal(err)
	}
	result, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != AddonRequeueInterval {
		t.Fatalf("ready addon requeue = %v", result.RequeueAfter)
	}
	var got rlarkv1alpha1.Addon
	if err := mgmt.Get(ctx, types.NamespacedName{Name: "csi", Namespace: "test"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != rlarkv1alpha1.AddonPhaseReady {
		t.Fatalf("expected Ready branch, got %s: %s", got.Status.Phase, got.Status.Message)
	}
	// Simulate the next scheduled pass after an external actor removed a
	// desired field without changing the management Addon's resourceVersion.
	var config corev1.ConfigMap
	if err := local.Get(ctx, types.NamespacedName{Name: "csi-driver-rclone-config", Namespace: "rlark-system"}, &config); err != nil {
		t.Fatal(err)
	}
	config.Data = map[string]string{"unexpected": "drift"}
	if err := local.Update(ctx, &config); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := local.Get(ctx, types.NamespacedName{Name: "csi-driver-rclone-config", Namespace: "rlark-system"}, &config); err != nil {
		t.Fatal(err)
	}
	if _, exists := config.Data["unexpected"]; exists {
		t.Fatal("scheduled reconciliation left drifted config")
	}
}

func TestApplyObjectInjectsDistributedImagePullSecrets(t *testing.T) {
	ctx := context.Background()
	credential := func(name, registry string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "rlark-system", Labels: map[string]string{"rlark.io/image-registry-credential": "true"}, Annotations: map[string]string{"rlark.io/registry": registry}},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte("{}")},
		}
	}
	cl := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(credential("ir-site", "registry.rlark.internal:5443"), credential("ir-other", "other.example:5000")).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "u", ResourceVersion: "1"}}
	objs, err := parseManifest([]byte(`apiVersion: apps/v1
kind: DaemonSet
metadata: {name: d, namespace: rlark-system}
spec:
  selector: {matchLabels: {app: d}}
  template:
    metadata: {labels: {app: d}}
    spec:
      containers: [{name: c, image: "registry.rlark.internal:5443/fleet/robot@sha256:0000000000000000000000000000000000000000000000000000000000000000"}]
`))
	if err != nil {
		t.Fatal(err)
	}
	injectLabels(objs[0], addon.Name, string(addon.UID))
	objs[0].SetAnnotations(map[string]string{AddonGenerationAnnotation: strconv.FormatInt(addon.Generation, 10)})
	if err := r.applyObject(ctx, objs[0], addon); err != nil {
		t.Fatal(err)
	}
	var got appsv1.DaemonSet
	if err := cl.Get(ctx, types.NamespacedName{Name: "d", Namespace: "rlark-system"}, &got); err != nil {
		t.Fatal(err)
	}
	if secrets := got.Spec.Template.Spec.ImagePullSecrets; len(secrets) != 1 || secrets[0].Name != "ir-site" {
		t.Fatalf("imagePullSecrets = %v, want [ir-site]", secrets)
	}

	// A credential that comes to match the unchanged Addon's images (live 11-17: the cloud's moved to the
	// registry's name) replaces none of its Pods; the Addon's next change takes it.
	if err := cl.Create(ctx, credential("ir-cloud", "registry.rlark.internal:5443")); err != nil {
		t.Fatal(err)
	}
	apply := func() appsv1.DaemonSet {
		t.Helper()
		again := objs[0].DeepCopy()
		unstructured.RemoveNestedField(again.Object, "spec", "template", "spec", "imagePullSecrets")
		again.SetAnnotations(map[string]string{AddonGenerationAnnotation: strconv.FormatInt(addon.Generation, 10)})
		if err := r.applyObject(ctx, again, addon); err != nil {
			t.Fatal(err)
		}
		var daemonSet appsv1.DaemonSet
		if err := cl.Get(ctx, types.NamespacedName{Name: "d", Namespace: "rlark-system"}, &daemonSet); err != nil {
			t.Fatal(err)
		}
		return daemonSet
	}
	if same := apply(); same.ResourceVersion != got.ResourceVersion {
		t.Fatalf("an unchanged Addon's DaemonSet was written: pulls with %v", same.Spec.Template.Spec.ImagePullSecrets)
	}
	addon.Generation++
	if changed := apply(); len(changed.Spec.Template.Spec.ImagePullSecrets) != 2 {
		t.Fatalf("after the Addon changed it pulls with %v", changed.Spec.Template.Spec.ImagePullSecrets)
	}

	// A Pod that is not ready may be waiting for the credential: the unchanged Addon takes it at once.
	waiting := apply()
	waiting.Status.DesiredNumberScheduled = 1
	if err := cl.Status().Update(ctx, &waiting); err != nil {
		t.Fatal(err)
	}
	if err := cl.Create(ctx, credential("ir-late", "registry.rlark.internal:5443")); err != nil {
		t.Fatal(err)
	}
	if healed := apply(); len(healed.Spec.Template.Spec.ImagePullSecrets) != 3 {
		t.Fatalf("with a Pod not ready it pulls with %v", healed.Spec.Template.Spec.ImagePullSecrets)
	}
}

func TestCheckReadyDaemonSetForAClassOfNodesMayHaveNone(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "u"}}
	daemonSet := func(name string, affinity *corev1.Affinity, observed int64) *appsv1.DaemonSet {
		return &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "rlark-system", Generation: 1, Labels: map[string]string{"rlark.io/addon-name": "a"}},
			Spec:       appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: affinity}}},
			Status:     appsv1.DaemonSetStatus{ObservedGeneration: observed},
		}
	}
	robots := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}
	for _, tt := range []struct {
		name  string
		ds    *appsv1.DaemonSet
		ready bool
	}{
		{"every node, none scheduled", daemonSet("all", nil, 1), false},
		{"robot nodes, none in cluster", daemonSet("robot", robots, 1), true},
		{"robot nodes, not yet observed", daemonSet("robot", robots, 0), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(tt.ds).WithStatusSubresource(tt.ds).Build()
			r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
			ready, msg, err := r.checkReady(ctx, addon)
			if err != nil || ready != tt.ready {
				t.Fatalf("checkReady = %v %q %v, want %v", ready, msg, err, tt.ready)
			}
		})
	}
}

func TestCheckReadyDeploymentStuckRolloutIsNotReady(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "u"}}
	one := int32(1)
	// The old ReplicaSet's Pod is ready, the new one cannot pull its image.
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "rlark-system", Generation: 2, Labels: map[string]string{"rlark.io/addon-name": "a"}},
		Spec:       appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "c"}}},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 2, UpdatedReplicas: 1, ReadyReplicas: 1},
	}
	cl := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(d).WithStatusSubresource(d).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	if ready, msg, err := r.checkReady(ctx, addon); err != nil || ready {
		t.Fatalf("checkReady = %v %q %v, want not ready", ready, msg, err)
	}
}

// A status write changes the management Addon's resourceVersion but not its generation. The local object,
// already rendered from that generation and unchanged, must not be written again: an annotation update
// moves a Deployment's generation, and the Addon would read as not ready on every reconciliation.
func TestApplyObjectLeavesAnUnchangedObjectAloneAcrossStatusWrites(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "site-registry", UID: types.UID("addon-uid"), Generation: 3, ResourceVersion: "1001"}}
	makeObject := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "site-registry", "namespace": "rlark-system", "labels": map[string]interface{}{"rlark.io/addon-name": "site-registry", "rlark.io/addon-uid": "addon-uid"}, "annotations": map[string]interface{}{AddonGenerationAnnotation: "3"}},
			"spec":     map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"hostNetwork": true}}},
		}}
	}
	cl := fake.NewClientBuilder().WithObjects(makeObject()).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	before := makeObject()
	if err := cl.Get(ctx, types.NamespacedName{Name: "site-registry", Namespace: "rlark-system"}, before); err != nil {
		t.Fatal(err)
	}
	addon.ResourceVersion = "1002"
	if err := r.applyObject(ctx, makeObject(), addon); err != nil {
		t.Fatal(err)
	}
	after := makeObject()
	if err := cl.Get(ctx, types.NamespacedName{Name: "site-registry", Namespace: "rlark-system"}, after); err != nil {
		t.Fatal(err)
	}
	if after.GetResourceVersion() != before.GetResourceVersion() {
		t.Fatalf("object written again after a status-only change: resourceVersion %s -> %s", before.GetResourceVersion(), after.GetResourceVersion())
	}
}

func TestCheckReadyDaemonSetNamesTheNodesItWaitsOn(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "u"}}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "trust", Namespace: "rlark-system", Generation: 1, Labels: map[string]string{"rlark.io/addon-name": "a"}},
		Spec:       appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "trust"}}},
		Status:     appsv1.DaemonSetStatus{ObservedGeneration: 1, DesiredNumberScheduled: 3, UpdatedNumberScheduled: 3, NumberReady: 1},
	}
	node := func(name string, ready corev1.ConditionStatus) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}}}
	}
	pod := func(name, nodeName string, ready corev1.ConditionStatus, waiting string) *corev1.Pod {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "rlark-system", Labels: map[string]string{"app": "trust"}},
			Spec:       corev1.PodSpec{NodeName: nodeName},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: ready}}},
		}
		if waiting != "" {
			p.Status.Phase = corev1.PodPending
			p.Status.ContainerStatuses = []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: waiting}}}}
		}
		return p
	}
	cl := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(ds,
		node("cailiang", corev1.ConditionTrue), node("nb260706069", corev1.ConditionUnknown), node("wendychen", corev1.ConditionTrue),
		pod("t1", "cailiang", corev1.ConditionTrue, ""), pod("t2", "nb260706069", corev1.ConditionFalse, ""), pod("t3", "wendychen", corev1.ConditionFalse, "ImagePullBackOff"),
	).WithStatusSubresource(ds).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	ready, msg, err := r.checkReady(ctx, addon)
	want := "DaemonSet trust: 1/3 ready; waiting on nb260706069 (node NotReady), wendychen (ImagePullBackOff)"
	if err != nil || ready || msg != want {
		t.Fatalf("checkReady = %v %q %v, want %q", ready, msg, err, want)
	}
}

// Moving a site's cache to another machine is moving the label its Deployment selects on: the Pod on the node
// that lost the label is replaced, the one on a matching node and Pods of other workloads are left alone.
func TestReplacePodsOnUnselectedNodesFollowsTheLabel(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "site-registry", Namespace: "rlark-system", Labels: map[string]string{"rlark.io/addon-name": "site-registry"}},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "site-registry"}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"rlark.io/site-registry": "true"}}}},
	}
	node := func(name string, labels map[string]string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
	}
	pod := func(name, app, nodeName string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "rlark-system", UID: types.UID(name), Labels: map[string]string{"app": app}}, Spec: corev1.PodSpec{NodeName: nodeName}}
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment,
		node("old", nil), node("new", map[string]string{"rlark.io/site-registry": "true"}),
		pod("cache-on-old", "site-registry", "old"), pod("cache-on-new", "site-registry", "new"), pod("other-on-old", "other", "old")).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	if err := r.replacePodsOnUnselectedNodes(ctx, &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "site-registry"}}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"cache-on-old": false, "cache-on-new": true, "other-on-old": true} {
		err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "rlark-system"}, &corev1.Pod{})
		if (err == nil) != want {
			t.Errorf("%s: present=%v, want %v", name, err == nil, want)
		}
	}
}

// An Addon whose cache is down says so in the operator's words, with the Pod's own reason: no one has to
// guess from an x509 or a connection refused on a node.
func TestCheckReadySaysWhyInTheWorkloadsWords(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "site-registry", Namespace: "rlark-system", Generation: 1,
			Labels:      map[string]string{"rlark.io/addon-name": "site-registry"},
			Annotations: map[string]string{NotReadyMessageAnnotation: "站点缓存不可用"}},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "site-registry"}}},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "site-registry-x", Namespace: "rlark-system", Labels: map[string]string{"app": "site-registry"}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "registry",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off 40s"}}}}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment, pod).Build()
	r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
	ready, msg, err := r.checkReady(ctx, &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "site-registry"}})
	want := "站点缓存不可用: Deployment site-registry: 1/1 updated, 0 ready; container registry CrashLoopBackOff: back-off 40s"
	if err != nil || ready || msg != want {
		t.Fatalf("checkReady = %v, %q, %v; want %q", ready, msg, err, want)
	}
}

// csi-driver-rclone on the cloud keeps its node plugins with OnDelete (a node's mounts are not cut by a new
// template): ready, old-template Pods are what was asked for. Under RollingUpdate they are an unfinished rollout.
func TestCheckReadyOnDeleteDaemonSetIsReadyWithOldPods(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "u"}}
	for _, tt := range []struct {
		strategy appsv1.DaemonSetUpdateStrategyType
		ready    bool
	}{{appsv1.OnDeleteDaemonSetStrategyType, true}, {appsv1.RollingUpdateDaemonSetStrategyType, false}} {
		ds := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "node", Namespace: "rlark-system", Generation: 2, Labels: map[string]string{"rlark.io/addon-name": "a"}},
			Spec:       appsv1.DaemonSetSpec{UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: tt.strategy}, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "node"}}},
			Status:     appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 2, NumberReady: 2, UpdatedNumberScheduled: 0},
		}
		cl := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(ds).WithStatusSubresource(ds).Build()
		r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
		ready, msg, err := r.checkReady(ctx, addon)
		if err != nil || ready != tt.ready {
			t.Fatalf("%s: checkReady = %v %q %v, want %v", tt.strategy, ready, msg, err, tt.ready)
		}
	}
}

// An Addon's DaemonSet lands as one RLark rolls: OnDelete, with the policy as its annotation. What the template
// says of a rolling update is the policy's bound; a template that says OnDelete without the annotation keeps its
// Pods until someone replaces them.
func TestAddonDaemonSetsAreRolledByRLark(t *testing.T) {
	daemonSet := func(annotations map[string]interface{}, strategy map[string]interface{}) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "DaemonSet",
			"metadata": map[string]interface{}{"name": "d"},
			"spec":     map[string]interface{}{},
		}}
		if annotations != nil {
			_ = unstructured.SetNestedMap(obj.Object, annotations, "metadata", "annotations")
		}
		if strategy != nil {
			_ = unstructured.SetNestedMap(obj.Object, strategy, "spec", "updateStrategy")
		}
		return obj
	}
	for _, test := range []struct {
		name     string
		obj      *unstructured.Unstructured
		policy   string
		strategy string
	}{
		{name: "nothing said", obj: daemonSet(nil, nil), policy: `{}`, strategy: "OnDelete"},
		{name: "a rolling update's bound", obj: daemonSet(nil, map[string]interface{}{"type": "RollingUpdate", "rollingUpdate": map[string]interface{}{"maxUnavailable": "100%"}}), policy: `{"maxUnavailable":"100%"}`, strategy: "OnDelete"},
		{name: "a number", obj: daemonSet(nil, map[string]interface{}{"rollingUpdate": map[string]interface{}{"maxUnavailable": int64(2)}}), policy: `{"maxUnavailable":2}`, strategy: "OnDelete"},
		{name: "the template's policy", obj: daemonSet(map[string]interface{}{rlarkv1alpha1.RolloutAnnotation: `{"whenIdle":true}`}, nil), policy: `{"whenIdle":true}`, strategy: "OnDelete"},
		{name: "replaced by hand", obj: daemonSet(nil, map[string]interface{}{"type": "OnDelete"}), policy: "", strategy: "OnDelete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := rollDaemonSet(test.obj); err != nil {
				t.Fatal(err)
			}
			strategy, _, _ := unstructured.NestedMap(test.obj.Object, "spec", "updateStrategy")
			if policy := test.obj.GetAnnotations()[rlarkv1alpha1.RolloutAnnotation]; policy != test.policy || strategy["type"] != test.strategy || strategy["rollingUpdate"] != nil {
				t.Fatalf("policy=%q strategy=%v", policy, strategy)
			}
		})
	}
	bad := daemonSet(map[string]interface{}{rlarkv1alpha1.RolloutAnnotation: `ten percent`}, nil)
	if err := rollDaemonSet(bad); err == nil {
		t.Fatal("a rollout annotation that is no policy was accepted")
	}
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "d"}}}
	if err := rollDaemonSet(deployment); err != nil || deployment.GetAnnotations() != nil {
		t.Fatalf("a Deployment was changed: %v %v", err, deployment.Object)
	}
}

// An Addon whose DaemonSet RLark rolls is ready when every node that is Ready runs the current template: a node
// that is off is not waited for, and with whenIdle neither is a node in use that still runs the old one.
func TestCheckReadyRolledDaemonSetCountsReadyNodesOnly(t *testing.T) {
	ctx := context.Background()
	addon := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "u"}}
	node := func(name string, ready corev1.ConditionStatus, inUse string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{rlarkv1alpha1.NodeInUseAnnotation: inUse}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}}}
	}
	pod := func(nodeName, generation string, ready corev1.ConditionStatus) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p-" + nodeName, Namespace: "rlark-system", Labels: map[string]string{"app": "robot", "pod-template-generation": generation}},
			Spec:       corev1.PodSpec{NodeName: nodeName},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: ready}}},
		}
	}
	for _, test := range []struct {
		name    string
		policy  string
		objects []client.Object
		want    string
	}{
		{name: "a node that is off, with the old Pod", policy: `{}`,
			objects: []client.Object{node("r1", corev1.ConditionTrue, ""), pod("r1", "2", corev1.ConditionTrue), node("r2", corev1.ConditionUnknown, ""), pod("r2", "1", corev1.ConditionFalse)}},
		{name: "a ready node still to be updated", policy: `{}`,
			objects: []client.Object{node("r1", corev1.ConditionTrue, ""), pod("r1", "2", corev1.ConditionTrue), node("r2", corev1.ConditionTrue, ""), pod("r2", "1", corev1.ConditionTrue)},
			want:    "DaemonSet robot: waiting on r2 (to be updated)"},
		{name: "whenIdle and a node in use", policy: `{"whenIdle":true}`,
			objects: []client.Object{node("r1", corev1.ConditionTrue, ""), pod("r1", "2", corev1.ConditionTrue), node("r2", corev1.ConditionTrue, "fleet-station"), pod("r2", "1", corev1.ConditionTrue)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ds := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "robot", Namespace: "rlark-system", Generation: 2, Labels: map[string]string{"rlark.io/addon-name": "a"},
					Annotations: map[string]string{rlarkv1alpha1.RolloutAnnotation: test.policy, appsv1.DeprecatedTemplateGeneration: "2"}},
				Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "robot"}},
					Template:       corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"rlark.io/robot-model": "s1u"}}},
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}},
				Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 1, NumberReady: 1},
			}
			cl := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(append(test.objects, ds)...).WithStatusSubresource(ds).Build()
			r := &pullReconciler{c: &Controller{Controller: base.Controller{LocalKubeClient: cl}}}
			ready, msg, err := r.checkReady(ctx, addon)
			if err != nil || ready != (test.want == "") || msg != test.want {
				t.Fatalf("checkReady = %v %q %v, want %q", ready, msg, err, test.want)
			}
		})
	}
}

// publishedBuiltIn is the Package of a built-in item as the control plane publishes it to a cluster, under version.
func publishedBuiltIn(t *testing.T, name, version, cluster string) *corev1.ConfigMap {
	t.Helper()
	item, ok := addons.Catalog.Get(name)
	if !ok {
		t.Fatalf("%s is not built in", name)
	}
	pkg := addons.PackageOf(item)
	pkg.Version = version
	return pkg.ConfigMap(cluster)
}

// robotPackage is a published Package of one ConfigMap that says which content it was rendered from.
func robotPackage(version, content string) *corev1.ConfigMap {
	return addons.Package{Name: "robot", Version: version, Files: map[string]string{
		"addon.yaml":   "name: robot\nversion: " + version + "\nparameters:\n- {name: image, type: string, required: true}\n",
		"profile.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: robot-profile}\ndata: {image: \"{{ .Values.image }}\", content: " + content + "}\n",
	}}.ConfigMap("test")
}

func addonTestClients(t *testing.T, management ...client.Object) (client.Client, client.Client, *pullReconciler) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	mgmt := fake.NewClientBuilder().WithScheme(scheme).WithObjects(management...).WithStatusSubresource(&rlarkv1alpha1.Addon{}).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	return mgmt, local, &pullReconciler{c: &Controller{Controller: base.Controller{ManagementClient: mgmt, LocalKubeClient: local, AgentType: string(rlarkv1alpha1.AgentTypeKubernetes)}}}
}

// An Addon is rendered from the Package the control plane published for its name and version, once for each of
// its revisions. What a later agent would make of it, or of a Package that has changed or gone, changes nothing
// of an Addon that has not changed itself; a new revision is rendered from its own Package.
func TestAddonIsRenderedFromItsPublishedPackageOncePerRevision(t *testing.T) {
	ctx := context.Background()
	a := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "robot", Namespace: "test", UID: "uid", Generation: 1, Finalizers: []string{AddonFinalizer}},
		Spec: rlarkv1alpha1.AddonSpec{AddonName: "robot", Version: "v1", Values: map[string]string{"image": "robot@sha256:1"}}}
	mgmt, local, r := addonTestClients(t, a, robotPackage("v1", "first"))
	req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(a)}
	profile := func() map[string]string {
		t.Helper()
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatal(err)
		}
		var config corev1.ConfigMap
		if err := local.Get(ctx, types.NamespacedName{Name: "robot-profile", Namespace: "rlark-system"}, &config); err != nil {
			t.Fatal(err)
		}
		return config.Data
	}
	if got := profile(); got["image"] != "robot@sha256:1" || got["content"] != "first" {
		t.Fatalf("rendered %v", got)
	}

	// The same revision, and its Package reads differently now (as a catalog compiled into another agent would).
	other := robotPackage("v1", "other")
	if err := mgmt.Delete(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := mgmt.Create(ctx, robotPackage("v1", "other")); err != nil {
		t.Fatal(err)
	}
	if got := profile(); got["content"] != "first" {
		t.Fatalf("an Addon that did not change was rendered again: %v", got)
	}
	// Nor does it need its Package any more, and what someone changed in the cluster is put back.
	if err := mgmt.Delete(ctx, robotPackage("v1", "other")); err != nil {
		t.Fatal(err)
	}
	var config corev1.ConfigMap
	if err := local.Get(ctx, types.NamespacedName{Name: "robot-profile", Namespace: "rlark-system"}, &config); err != nil {
		t.Fatal(err)
	}
	config.Data = map[string]string{"content": "drift"}
	if err := local.Update(ctx, &config); err != nil {
		t.Fatal(err)
	}
	if got := profile(); got["content"] != "first" || got["image"] != "robot@sha256:1" {
		t.Fatalf("without its Package the Addon's object reads %v", got)
	}

	// A new revision: the next version, from that version's Package.
	if err := mgmt.Create(ctx, robotPackage("v2", "second")); err != nil {
		t.Fatal(err)
	}
	var current rlarkv1alpha1.Addon
	if err := mgmt.Get(ctx, req.NamespacedName, &current); err != nil {
		t.Fatal(err)
	}
	current.Spec.Version, current.Generation = "v2", 2
	if err := mgmt.Update(ctx, &current); err != nil {
		t.Fatal(err)
	}
	if got := profile(); got["content"] != "second" {
		t.Fatalf("the new revision was not rendered from its Package: %v", got)
	}
}

// A revision whose Package was not published is not rendered from anything else. An Addon that is as it was (the
// agent was replaced) keeps its objects and its phase and says what is missing; a new revision has failed.
func TestAddonWithoutItsPackageIsLeftAsItIs(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name     string
		observed int64
		phase    rlarkv1alpha1.AddonPhase
		want     rlarkv1alpha1.AddonPhase
	}{
		{name: "as it was", observed: 3, phase: rlarkv1alpha1.AddonPhaseReady, want: rlarkv1alpha1.AddonPhaseReady},
		{name: "a new revision", observed: 2, phase: rlarkv1alpha1.AddonPhaseReady, want: rlarkv1alpha1.AddonPhaseFailed},
		{name: "never installed", want: rlarkv1alpha1.AddonPhaseFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			// csi-driver-rclone is in the catalog compiled into this agent, which is not where an Addon is rendered from.
			a := &rlarkv1alpha1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "csi-driver-rclone", Namespace: "test", UID: "uid", Generation: 3, Finalizers: []string{AddonFinalizer}},
				Spec:   rlarkv1alpha1.AddonSpec{AddonName: "csi-driver-rclone", Version: "v0.1.0"},
				Status: rlarkv1alpha1.AddonStatus{Phase: test.phase, ObservedGeneration: test.observed}}
			mgmt, local, r := addonTestClients(t, a)
			running := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "csi-driver-rclone-node", Namespace: "rlark-system", Labels: map[string]string{"rlark.io/addon-name": "csi-driver-rclone", "rlark.io/addon-uid": "uid"}},
				Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "rclone", Image: "as-it-runs"}}}}}}
			if err := local.Create(ctx, running); err != nil {
				t.Fatal(err)
			}
			before := running.ResourceVersion
			if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(a)}); err != nil {
				t.Fatal(err)
			}
			var got rlarkv1alpha1.Addon
			if err := mgmt.Get(ctx, client.ObjectKeyFromObject(a), &got); err != nil {
				t.Fatal(err)
			}
			if got.Status.Phase != test.want || !strings.HasPrefix(got.Status.Message, "PackageMissing: version \"v0.1.0\" of addon \"csi-driver-rclone\"") {
				t.Fatalf("phase=%s message=%q", got.Status.Phase, got.Status.Message)
			}
			var daemonSets appsv1.DaemonSetList
			var deployments appsv1.DeploymentList
			if err := local.List(ctx, &daemonSets); err != nil {
				t.Fatal(err)
			}
			if err := local.List(ctx, &deployments); err != nil {
				t.Fatal(err)
			}
			if len(daemonSets.Items) != 1 || daemonSets.Items[0].ResourceVersion != before || len(deployments.Items) != 0 {
				t.Fatalf("the Addon's objects were touched: %d DaemonSets (resourceVersion %s, was %s), %d Deployments", len(daemonSets.Items), daemonSets.Items[0].ResourceVersion, before, len(deployments.Items))
			}
		})
	}
}
