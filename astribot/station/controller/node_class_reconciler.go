package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// nodeClassLabelsAnnotation lists, on a management Node, the label keys
	// its class gave it, so that a key the class drops is taken off again.
	nodeClassLabelsAnnotation = "station.rlark.io/node-class-labels"
)

// NodeClassReconciler keeps every node of a class at what the class declares
// now, not at what it declared when the node came in: the admission, which a
// machine installed anew registers with, and the labels of the management
// Node, which the cluster agent carries to the cluster's node and puts back
// when they are changed there. Taints are only what a node registers with.
type NodeClassReconciler struct {
	Client client.Client
	Reader client.Reader
}

func (r *NodeClassReconciler) SetupWithManager(mgr manager.Manager) error {
	ofClass := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, object client.Object) []reconcile.Request {
		if class := object.(*stationv1alpha1.NodeAdmission).Spec.NodeClass; class != "" {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: class}}}
		}
		return nil
	})
	return ctrl.NewControllerManagedBy(mgr).Named("node-class").
		For(&stationv1alpha1.NodeClass{}).
		Watches(&stationv1alpha1.NodeAdmission{}, ofClass).
		Complete(r)
}

func (r *NodeClassReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var class stationv1alpha1.NodeClass
	if err := r.Reader.Get(ctx, types.NamespacedName{Name: req.Name}, &class); apierrors.IsNotFound(err) {
		// Its nodes stay as they are; a class is removed when none are left.
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read NodeClass %s: %w", req.Name, err)
	}
	var admissions stationv1alpha1.NodeAdmissionList
	if err := r.Reader.List(ctx, &admissions); err != nil {
		return reconcile.Result{}, fmt.Errorf("list the NodeAdmissions of NodeClass %s: %w", class.Name, err)
	}
	for i := range admissions.Items {
		admission := &admissions.Items[i]
		if admission.Spec.NodeClass != class.Name {
			continue
		}
		// The labels the machine has values of its own for (its number, a
		// body that is not the usual one) stay the machine's.
		own := strings.Split(admission.Annotations[machineLabelsAnnotation], ",")
		wanted := map[string]string{}
		for key, value := range admission.Spec.Labels {
			if slices.Contains(own, key) {
				wanted[key] = value
			}
		}
		for key, value := range class.Spec.Labels {
			if !slices.Contains(own, key) {
				wanted[key] = value
			}
		}
		if !reflect.DeepEqual(admission.Spec.Labels, wanted) || !reflect.DeepEqual(admission.Spec.Taints, class.Spec.Taints) {
			admission.Spec.Labels, admission.Spec.Taints = wanted, class.Spec.Taints
			if err := r.Client.Update(ctx, admission); err != nil {
				return reconcile.Result{}, fmt.Errorf("bring NodeAdmission %s/%s to NodeClass %s: %w", admission.Namespace, admission.Name, class.Name, err)
			}
		}
		node := &unstructured.Unstructured{}
		node.SetGroupVersionKind(rlarkNodeGVK)
		if err := r.Reader.Get(ctx, client.ObjectKey{Namespace: admission.Spec.ClusterID, Name: admission.Name}, node); apierrors.IsNotFound(err) {
			continue // the machine has not joined yet
		} else if err != nil {
			return reconcile.Result{}, fmt.Errorf("read RLark Node %s/%s: %w", admission.Spec.ClusterID, admission.Name, err)
		}
		declared := strings.Join(slices.Sorted(maps.Keys(wanted)), ",")
		labels := map[string]any{}
		for _, key := range strings.Split(node.GetAnnotations()[nodeClassLabelsAnnotation], ",") {
			if _, still := wanted[key]; key != "" && !still && node.GetLabels()[key] != "" {
				labels[key] = nil
			}
		}
		for key, value := range wanted {
			if node.GetLabels()[key] != value {
				labels[key] = value
			}
		}
		if len(labels) == 0 && node.GetAnnotations()[nodeClassLabelsAnnotation] == declared {
			continue
		}
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"labels": labels, "annotations": map[string]any{nodeClassLabelsAnnotation: declared}}})
		if err := r.Client.Patch(ctx, node, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return reconcile.Result{}, fmt.Errorf("label RLark Node %s/%s as NodeClass %s: %w", admission.Spec.ClusterID, admission.Name, class.Name, err)
		}
	}
	// A label changed by hand on a management Node is put back at the next pass.
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}
