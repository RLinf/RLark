package deviceplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/mutatingwebhook"
)

// ---------------------------------------------------------------------------
// WebhookConfig — CLI-derived options for the mutating webhook that
// auto-injects the devinit init container. This is intentionally NOT part of
// PluginConfig: it is an operator-side deployment concern (which Service
// fronts the webhook, which MutatingWebhookConfiguration to manage, where the
// CA Secret lives) rather than a node-level device concern. The device-plugin
// CLI assembles it from flags and passes it to NewPlugin.
//
// The webhook is only meaningful alongside host_macvlans: it injects the init
// container that runs `devinit setup`, which triggers macvlan creation in the
// pod's network namespace. With no host_macvlans the device plugin does not
// start the webhook even when Enabled is true.
// ---------------------------------------------------------------------------

// WebhookConfig holds the mutating webhook configuration passed via CLI flags.
type WebhookConfig struct {
	// Enabled starts the mutating webhook server.
	Enabled bool

	// Addr is the HTTPS listen address. Defaults to ":9443".
	Addr string

	// Path is the admission endpoint path. Defaults to "/mutate".
	Path string

	// MutatingWebhookConfigName is the name of the MutatingWebhookConfiguration
	// whose caBundle is auto-managed. Required when Enabled.
	MutatingWebhookConfigName string

	// ServiceName and Namespace identify the Kubernetes Service that fronts
	// the webhook. They form the DNS Subject Alternative Names of the serving
	// certificate so the API server can verify it. Required when Enabled.
	ServiceName string
	Namespace   string

	// CASecretName and CASecretNamespace, when set, persist the CA (cert+key)
	// in a Secret so the same CA survives restarts. When empty the CA is
	// in-memory and regenerated on every start.
	CASecretName      string
	CASecretNamespace string

	// DevinitImage is the init container image. It must contain the devinit
	// binary at devinitBinaryPath. When empty, NewPlugin fills it from the
	// auto-discovered device-plugin image (downward API).
	DevinitImage string
}

// DefaultWebhookConfig returns a WebhookConfig with sensible defaults.
func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		Addr: ":9443",
		Path: "/mutate",
	}
}

// EffectiveAddr returns Addr, defaulting to ":9443".
func (w WebhookConfig) EffectiveAddr() string {
	if w.Addr != "" {
		return w.Addr
	}
	return ":9443"
}

// EffectivePath returns Path, defaulting to "/mutate".
func (w WebhookConfig) EffectivePath() string {
	if w.Path != "" {
		return w.Path
	}
	return "/mutate"
}

// dnsNames returns the DNS SANs for the serving certificate, derived from the
// Service name and namespace. When either is empty it falls back to localhost
// (useful only for local testing since the API server needs a real Service).
func (w WebhookConfig) dnsNames() []string {
	if w.ServiceName == "" || w.Namespace == "" {
		return []string{"localhost"}
	}
	return []string{
		w.ServiceName,
		w.ServiceName + "." + w.Namespace,
		w.ServiceName + "." + w.Namespace + ".svc",
		w.ServiceName + "." + w.Namespace + ".svc.cluster.local",
	}
}

// ---------------------------------------------------------------------------
// Init-container injection constants
// ---------------------------------------------------------------------------

const (
	// devinitContainerName is the name of the init container injected by the
	// webhook. Reused to detect an existing injection (idempotency).
	devinitContainerName = "rlark-devinit"

	// devinitBinaryPath is the devinit CLI path inside the init container
	// image. The device-plugin image ships it here (see the Dockerfile).
	devinitBinaryPath = "/usr/local/bin/devinit"
)

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// devinitHandler is the mutating webhook handler that injects the devinit
// init container into pods requesting this plugin's resource.
//
// The injected init container declares the same extended resource as the
// workload container. That is sufficient: the device plugin's Allocate
// (called for every container that requests the resource) injects the
// RunDir mount (which exposes the devinit service socket) and the
// RLINF_EMBODIED_DEVINIT_SOCKET_PATH env var, so the init container can
// locate the socket and run `devinit setup`. No extra volumes or mounts are
// needed from the webhook. The devinit binary itself lives in the image.
//
// The init container runs `devinit setup`, which dials the device plugin's
// init service Unix socket; the service reads the caller's PID from the
// socket peer credentials and creates the configured macvlans in that PID's
// network namespace (skipped for hostNetwork pods).
type devinitHandler struct {
	resourceName string // extended resource advertised by this plugin
	image        string // init container image (contains devinit binary)
}

// newDevinitHandler builds a handler from the plugin's resolved configuration.
func newDevinitHandler(resourceName, image string) *devinitHandler {
	return &devinitHandler{resourceName: resourceName, image: image}
}

// Mutate implements mutatingwebhook.Handler.
func (h *devinitHandler) Mutate(_ context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// Only Pods on CREATE/UPDATE; the API server should already filter by the
	// webhook's rules, but guard defensively.
	if req.Kind.Kind != "Pod" {
		return &admissionv1.AdmissionResponse{Allowed: true}
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		log.Printf("[device-plugin/webhook] decode pod: %v", err)
		return deny(req.UID, fmt.Errorf("decode pod: %w", err))
	}

	// Only inject when the pod actually requests this plugin's resource.
	if !podRequestsResource(&pod, h.resourceName) {
		return &admissionv1.AdmissionResponse{Allowed: true, UID: req.UID}
	}

	// Idempotency: if a devinit init container is already present (a prior
	// injection, or an operator who added one manually) leave the pod alone.
	if hasInitContainer(&pod, devinitContainerName) {
		return &admissionv1.AdmissionResponse{Allowed: true, UID: req.UID}
	}

	patch, err := buildDevinitPatch(&pod, h.resourceName, h.image)
	if err != nil {
		log.Printf("[device-plugin/webhook] build patch: %v", err)
		return deny(req.UID, err)
	}
	pt := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		UID:       req.UID,
		Patch:     patch,
		PatchType: &pt,
	}
}

// deny returns a denied AdmissionResponse carrying the error message.
func deny(uid types.UID, err error) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		UID:     uid,
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: err.Error(),
		},
	}
}

// ---------------------------------------------------------------------------
// Patch construction
// ---------------------------------------------------------------------------

// patchOp is a single RFC 6902 JSON Patch operation.
type patchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// buildDevinitPatch returns the JSON Patch that appends the devinit init
// container to spec.initContainers. The container declares the same extended
// resource as the workload; the device plugin's Allocate then injects the
// socket mount and discovery env vars — no volumes are touched by the webhook.
// On CREATE the array may be absent, so the first add creates it; on UPDATE it
// already exists, so the append form (/spec/initContainers/-) is used.
func buildDevinitPatch(pod *corev1.Pod, resourceName, image string) ([]byte, error) {
	container := buildDevinitContainer(resourceName, image)

	var ops []patchOp
	if len(pod.Spec.InitContainers) == 0 {
		ops = append(ops, patchOp{Op: "add", Path: "/spec/initContainers", Value: []corev1.Container{container}})
	} else {
		ops = append(ops, patchOp{Op: "add", Path: "/spec/initContainers/-", Value: container})
	}
	return json.Marshal(ops)
}

// buildDevinitContainer constructs the init container spec. It requests one
// unit of the plugin's extended resource (so Allocate injects the RunDir
// socket mount and RLINF_EMBODIED_DEVINIT_SOCKET_PATH env var) and runs the
// devinit CLI, which lives in the image at devinitBinaryPath.
//
// The extended resource is mirrored in limits: Kubernetes requires
// extended-resource limits to equal their requests, and clusters enforcing a
// LimitRanger / ResourceQuota reject containers — including init containers —
// that declare no limits, which would otherwise block the injected pod from
// being created.
func buildDevinitContainer(resourceName, image string) corev1.Container {
	resName := corev1.ResourceName(resourceName)
	one := resource.MustParse("1")
	return corev1.Container{
		Name:    devinitContainerName,
		Image:   image,
		Command: []string{devinitBinaryPath, "setup"},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{resName: one},
			Limits:   corev1.ResourceList{resName: one},
		},
	}
}

// ---------------------------------------------------------------------------
// Pod inspection helpers
// ---------------------------------------------------------------------------

// podRequestsResource reports whether any container (init or app) in the pod
// requests the given extended resource.
func podRequestsResource(pod *corev1.Pod, resourceName string) bool {
	res := corev1.ResourceName(resourceName)
	for i := range pod.Spec.InitContainers {
		if _, ok := pod.Spec.InitContainers[i].Resources.Requests[res]; ok {
			return true
		}
	}
	for i := range pod.Spec.Containers {
		if _, ok := pod.Spec.Containers[i].Resources.Requests[res]; ok {
			return true
		}
	}
	return false
}

// hasInitContainer reports whether the pod already has an init container with
// the given name (idempotency check).
func hasInitContainer(pod *corev1.Pod, name string) bool {
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return true
		}
	}
	return false
}

// Compile-time assertion that devinitHandler satisfies the webhook Handler.
var _ mutatingwebhook.Handler = (*devinitHandler)(nil)

// ---------------------------------------------------------------------------
// Server construction
// ---------------------------------------------------------------------------

// newWebhookServer builds the mutating webhook server from the plugin's
// resolved configuration. It returns (nil, nil) when the webhook is disabled
// or when host_macvlans is empty (there is nothing to inject). It returns an
// error when required fields are missing so the caller can log a warning and
// continue without the webhook rather than failing the whole device plugin.
//
// The devinit image is resolved in this order: WebhookConfig.DevinitImage
// (CLI flag), then the auto-discovered device-plugin image (downward API),
// which is the natural default since the device-plugin image ships the
// devinit binary.
func newWebhookServer(p *Plugin) (*mutatingwebhook.Server, error) {
	wh := p.webhookCfg
	if !wh.Enabled {
		return nil, nil
	}
	if len(p.cfg.HostMacvlans) == 0 {
		log.Printf("[device-plugin/webhook] enabled but no host_macvlans configured — skipping (nothing to inject)")
		return nil, nil
	}
	if p.clientset == nil {
		return nil, fmt.Errorf("no kubernetes clientset (need API access for caBundle management)")
	}
	if wh.MutatingWebhookConfigName == "" {
		return nil, fmt.Errorf("mutating webhook config name not set")
	}
	image := wh.DevinitImage
	if image == "" {
		image = p.disc.initImage
	}
	if image == "" {
		return nil, fmt.Errorf("devinit image not set and device-plugin image could not be auto-discovered")
	}
	cfg := mutatingwebhook.ServerConfig{
		Addr:              wh.EffectiveAddr(),
		Path:              wh.EffectivePath(),
		WebhookName:       wh.MutatingWebhookConfigName,
		Clientset:         p.clientset,
		Handler:           newDevinitHandler(p.cfg.EffectiveResourceName(), image),
		DNSNames:          wh.dnsNames(),
		CASecretName:      wh.CASecretName,
		CASecretNamespace: wh.CASecretNamespace,
		OwnerReferences:   p.disc.ownerRefs,
	}
	return mutatingwebhook.NewServer(cfg)
}
