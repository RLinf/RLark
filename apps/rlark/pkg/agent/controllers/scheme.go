package controllers

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// MgmtScheme is used for the management cluster manager, includes RLark CRDs + standard K8s types.
var MgmtScheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(MgmtScheme))
	utilruntime.Must(appsv1.AddToScheme(MgmtScheme))
	utilruntime.Must(rlarkv1alpha1.AddToScheme(MgmtScheme))
	utilruntime.Must(corev1.AddToScheme(MgmtScheme))
}
