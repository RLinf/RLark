package deviceplugin

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
)

const testResource = "rlinf.io/device"

// podWithResource builds a Pod whose sole app container requests one unit of
// the given extended resource.
func podWithResource(resourceName string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "app:v1",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName(resourceName): resource.MustParse("1"),
						},
					},
				},
			},
		},
	}
}

func rawJSON(t *testing.T, obj interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// Pod inspection helpers
// ---------------------------------------------------------------------------

func TestPodRequestsResource(t *testing.T) {
	pod := podWithResource(testResource)
	if !podRequestsResource(pod, testResource) {
		t.Error("app container requests the resource, want true")
	}
	if podRequestsResource(pod, "rlinf.io/device-wrong") {
		t.Error("matched a different resource, want false")
	}

	// Resource declared on an init container also counts.
	pod2 := podWithResource(testResource)
	pod2.Spec.Containers = []corev1.Container{{Name: "app", Image: "app:v1"}}
	pod2.Spec.InitContainers = []corev1.Container{{
		Name: "setup",
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceName(testResource): resource.MustParse("1")},
		},
	}}
	if !podRequestsResource(pod2, testResource) {
		t.Error("init container requests the resource, want true")
	}

	// No requests at all.
	empty := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}}
	if podRequestsResource(empty, testResource) {
		t.Error("empty requests matched, want false")
	}
}

func TestHasInitContainer(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "other"}},
	}}
	if hasInitContainer(pod, devinitContainerName) {
		t.Error("found devinit container where there is none")
	}
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{Name: devinitContainerName})
	if !hasInitContainer(pod, devinitContainerName) {
		t.Error("did not find devinit container")
	}
}

// ---------------------------------------------------------------------------
// Container / patch construction
// ---------------------------------------------------------------------------

func TestBuildDevinitContainer(t *testing.T) {
	c := buildDevinitContainer(testResource, "rlinf/device-plugin:v1")
	if c.Name != devinitContainerName {
		t.Errorf("Name = %q, want %q", c.Name, devinitContainerName)
	}
	if c.Image != "rlinf/device-plugin:v1" {
		t.Errorf("Image = %q", c.Image)
	}
	if len(c.Command) != 2 || c.Command[0] != devinitBinaryPath || c.Command[1] != "setup" {
		t.Errorf("Command = %v, want [%q setup]", c.Command, devinitBinaryPath)
	}
	got := c.Resources.Requests[corev1.ResourceName(testResource)]
	if got.Value() != 1 {
		t.Errorf("resource request = %d, want 1", got.Value())
	}
	// The extended resource must be mirrored in limits: Kubernetes requires
	// extended-resource limits to equal their requests, and clusters with a
	// LimitRanger / ResourceQuota reject containers without limits, which
	// would block the injected pod from being created.
	lim, ok := c.Resources.Limits[corev1.ResourceName(testResource)]
	if !ok || lim.Value() != 1 {
		t.Errorf("resource limit = %v ok=%v, want 1", lim, ok)
	}
	if len(c.Env) != 0 {
		t.Errorf("Env should be empty (Allocate injects socket env), got %d", len(c.Env))
	}
	if len(c.VolumeMounts) != 0 {
		t.Errorf("VolumeMounts should be empty (Allocate handles mounts), got %d", len(c.VolumeMounts))
	}
}

func TestBuildDevinitPatch_CreatesArray(t *testing.T) {
	pod := podWithResource(testResource) // no init containers
	patch, err := buildDevinitPatch(pod, testResource, "img:v1")
	if err != nil {
		t.Fatalf("buildDevinitPatch: %v", err)
	}
	var ops []patchOp
	if err := json.Unmarshal(patch, &ops); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	if ops[0].Op != "add" || ops[0].Path != "/spec/initContainers" {
		t.Errorf("op = %s %s, want add /spec/initContainers", ops[0].Op, ops[0].Path)
	}
	// The value is an array of one container when the array is created.
	var arr []corev1.Container
	if err := jsonMarshalRoundtrip(t, ops[0].Value, &arr); err != nil {
		t.Fatalf("decode container array: %v", err)
	}
	if len(arr) != 1 || arr[0].Name != devinitContainerName {
		t.Errorf("container array = %+v", arr)
	}
}

func TestBuildDevinitPatch_Appends(t *testing.T) {
	pod := podWithResource(testResource)
	pod.Spec.InitContainers = []corev1.Container{{Name: "preexisting"}}
	patch, err := buildDevinitPatch(pod, testResource, "img:v1")
	if err != nil {
		t.Fatalf("buildDevinitPatch: %v", err)
	}
	var ops []patchOp
	if err := json.Unmarshal(patch, &ops); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	if ops[0].Op != "add" || ops[0].Path != "/spec/initContainers/-" {
		t.Errorf("op = %s %s, want add /spec/initContainers/-", ops[0].Op, ops[0].Path)
	}
	// The value is a single container when appending.
	var c corev1.Container
	if err := jsonMarshalRoundtrip(t, ops[0].Value, &c); err != nil {
		t.Fatalf("decode container: %v", err)
	}
	if c.Name != devinitContainerName {
		t.Errorf("container name = %q, want %q", c.Name, devinitContainerName)
	}
}

// jsonMarshalRoundtrip re-marshals then unmarshals a patch value (which comes
// back from json.Unmarshal as map[string]interface{}) into a typed target so
// the test can assert on typed fields.
func jsonMarshalRoundtrip(t *testing.T, value interface{}, target interface{}) error {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

// ---------------------------------------------------------------------------
// Handler end-to-end
// ---------------------------------------------------------------------------

func TestHandlerMutate_Injects(t *testing.T) {
	h := newDevinitHandler(testResource, "img:v1")
	pod := podWithResource(testResource)
	req := &admissionv1.AdmissionRequest{
		UID:       types.UID("abc"),
		Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: rawJSON(t, pod)},
	}
	resp := h.Mutate(context.Background(), req)
	if !resp.Allowed {
		t.Fatal("Allowed = false, want true")
	}
	if resp.UID != types.UID("abc") {
		t.Errorf("UID = %q, want abc", resp.UID)
	}
	if resp.PatchType == nil || *resp.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Fatalf("PatchType = %v, want %s", resp.PatchType, admissionv1.PatchTypeJSONPatch)
	}
	if len(resp.Patch) == 0 {
		t.Fatal("Patch is empty")
	}
}

func TestHandlerMutate_NoResource(t *testing.T) {
	h := newDevinitHandler(testResource, "img:v1")
	// Pod that does NOT request the resource.
	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "task"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "app:v1"}}},
	}
	req := &admissionv1.AdmissionRequest{
		UID:       types.UID("abc"),
		Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: rawJSON(t, pod)},
	}
	resp := h.Mutate(context.Background(), req)
	if !resp.Allowed {
		t.Fatal("Allowed = false, want true")
	}
	if len(resp.Patch) != 0 {
		t.Errorf("Patch should be empty, got %s", resp.Patch)
	}
	if resp.PatchType != nil {
		t.Error("PatchType should be nil when no patch")
	}
}

func TestHandlerMutate_Idempotent(t *testing.T) {
	h := newDevinitHandler(testResource, "img:v1")
	pod := podWithResource(testResource)
	// Pre-add the devinit container so the handler should skip.
	pod.Spec.InitContainers = []corev1.Container{{Name: devinitContainerName}}
	req := &admissionv1.AdmissionRequest{
		UID:       types.UID("abc"),
		Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: rawJSON(t, pod)},
	}
	resp := h.Mutate(context.Background(), req)
	if !resp.Allowed {
		t.Fatal("Allowed = false, want true")
	}
	if len(resp.Patch) != 0 {
		t.Errorf("Patch should be empty (idempotent), got %s", resp.Patch)
	}
}

func TestHandlerMutate_NonPodKind(t *testing.T) {
	h := newDevinitHandler(testResource, "img:v1")
	req := &admissionv1.AdmissionRequest{
		UID:       types.UID("abc"),
		Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{}`)},
	}
	resp := h.Mutate(context.Background(), req)
	if !resp.Allowed {
		t.Fatal("non-Pod should be allowed without injection")
	}
	if len(resp.Patch) != 0 {
		t.Error("non-Pod should not get a patch")
	}
}

func TestHandlerMutate_BadObject(t *testing.T) {
	h := newDevinitHandler(testResource, "img:v1")
	req := &admissionv1.AdmissionRequest{
		UID:       types.UID("abc"),
		Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte("not json")},
	}
	resp := h.Mutate(context.Background(), req)
	if resp.Allowed {
		t.Fatal("Allowed = true, want false (bad object)")
	}
	if resp.Result == nil {
		t.Fatal("Result (status) should be set on denial")
	}
}

// ---------------------------------------------------------------------------
// newWebhookServer construction
// ---------------------------------------------------------------------------

// pluginForWebhookTest assembles a Plugin with the given webhook config and a
// fake clientset pre-loaded with an empty-caBundle MutatingWebhookConfiguration.
func pluginForWebhookTest(t *testing.T, wh WebhookConfig, withClientset bool) *Plugin {
	t.Helper()
	var cs kubernetes.Interface
	if withClientset {
		cs = fake.NewSimpleClientset(&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: wh.MutatingWebhookConfigName},
			Webhooks:   []admissionregistrationv1.MutatingWebhook{{Name: "mutate.example.com"}},
		})
	}
	return &Plugin{
		cfg: PluginConfig{
			HostMacvlans: []netmac.MACVLANConfig{{Name: "macvlan0", HostNIC: "eno1", IP: "172.16.0.100/24"}},
		},
		clientset:  cs,
		webhookCfg: wh,
	}
}

func TestNewWebhookServer_Disabled(t *testing.T) {
	p := pluginForWebhookTest(t, WebhookConfig{}, false)
	s, err := newWebhookServer(p)
	if err != nil {
		t.Fatalf("expected nil error when disabled, got %v", err)
	}
	if s != nil {
		t.Fatal("expected nil server when disabled")
	}
}

func TestNewWebhookServer_NoMacvlan(t *testing.T) {
	wh := WebhookConfig{Enabled: true, MutatingWebhookConfigName: "wh", ServiceName: "svc", Namespace: "ns", DevinitImage: "img:v1"}
	p := &Plugin{cfg: PluginConfig{}, webhookCfg: wh} // no HostMacvlans, no clientset
	s, err := newWebhookServer(p)
	if err != nil {
		t.Fatalf("expected nil error when no macvlan, got %v", err)
	}
	if s != nil {
		t.Fatal("expected nil server when no host_macvlans")
	}
}

func TestNewWebhookServer_NoClientset(t *testing.T) {
	wh := WebhookConfig{Enabled: true, MutatingWebhookConfigName: "wh", ServiceName: "svc", Namespace: "ns", DevinitImage: "img:v1"}
	p := pluginForWebhookTest(t, wh, false) // clientset nil
	if _, err := newWebhookServer(p); err == nil {
		t.Fatal("expected error when no clientset, got nil")
	}
}

func TestNewWebhookServer_NoConfigName(t *testing.T) {
	wh := WebhookConfig{Enabled: true, ServiceName: "svc", Namespace: "ns", DevinitImage: "img:v1"}
	p := pluginForWebhookTest(t, wh, true)
	if _, err := newWebhookServer(p); err == nil {
		t.Fatal("expected error when MutatingWebhookConfigName empty, got nil")
	}
}

func TestNewWebhookServer_NoImage(t *testing.T) {
	wh := WebhookConfig{Enabled: true, MutatingWebhookConfigName: "wh", ServiceName: "svc", Namespace: "ns"}
	p := pluginForWebhookTest(t, wh, true) // no DevinitImage, no discovered initImage
	if _, err := newWebhookServer(p); err == nil {
		t.Fatal("expected error when no devinit image, got nil")
	}
}

func TestNewWebhookServer_OK(t *testing.T) {
	wh := WebhookConfig{
		Enabled:                   true,
		MutatingWebhookConfigName: "wh.example.com",
		ServiceName:               "device-plugin",
		Namespace:                 "rlark",
		DevinitImage:              "rlinf/device-plugin:v1",
	}
	p := pluginForWebhookTest(t, wh, true)
	s, err := newWebhookServer(p)
	if err != nil {
		t.Fatalf("newWebhookServer: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

// TestNewWebhookServer_AutoDiscoversImage confirms that when DevinitImage is
// empty, the discovered device-plugin image is used as the devinit image.
func TestNewWebhookServer_AutoDiscoversImage(t *testing.T) {
	wh := WebhookConfig{
		Enabled:                   true,
		MutatingWebhookConfigName: "wh.example.com",
		ServiceName:               "device-plugin",
		Namespace:                 "rlark",
	}
	p := pluginForWebhookTest(t, wh, true)
	p.disc.initImage = "auto/discovered:v1"
	s, err := newWebhookServer(p)
	if err != nil {
		t.Fatalf("newWebhookServer: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil server with auto-discovered image")
	}
}

// ---------------------------------------------------------------------------
// WebhookConfig helpers
// ---------------------------------------------------------------------------

func TestWebhookConfigDefaults(t *testing.T) {
	w := WebhookConfig{}
	if w.EffectiveAddr() != ":9443" {
		t.Errorf("EffectiveAddr = %q, want :9443", w.EffectiveAddr())
	}
	if w.EffectivePath() != "/mutate" {
		t.Errorf("EffectivePath = %q, want /mutate", w.EffectivePath())
	}
	if got := w.dnsNames(); len(got) != 1 || got[0] != "localhost" {
		t.Errorf("dnsNames = %v, want [localhost]", got)
	}

	w.ServiceName = "svc"
	w.Namespace = "rlark"
	names := w.dnsNames()
	want := []string{"svc", "svc.rlark", "svc.rlark.svc", "svc.rlark.svc.cluster.local"}
	if len(names) != len(want) {
		t.Fatalf("dnsNames = %v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("dnsNames[%d] = %q, want %q", i, n, want[i])
		}
	}
}
