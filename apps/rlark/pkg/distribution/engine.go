package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Engine struct {
	DeclarationClient client.Client
	TargetClient      dynamic.Interface
	TargetMapper      meta.RESTMapper
	Namespaces        NamespaceResolver
	InventoryStore    InventoryStore
	Policy            Policy
	Mode              Mode
	MaxTargets        int
}

type desiredObject struct {
	object  *unstructured.Unstructured
	mapping *meta.RESTMapping
	target  NamespaceIdentity
	config  Config
}

func (e *Engine) Reconcile(ctx context.Context, declaration *corev1.Secret) (Result, error) {
	if declaration.Type != e.Mode.SecretType {
		return Result{}, nil
	}
	inventory, err := e.InventoryStore.Load(ctx, declaration)
	if err != nil {
		return Result{}, err
	}
	if declaration.DeletionTimestamp != nil {
		return e.cleanup(ctx, declaration, inventory)
	}
	definition, err := Parse(declaration.Data)
	if err != nil {
		return e.fail(ctx, declaration, inventory, "InvalidDefinition", err)
	}
	desiredObjects, phase, err := e.expand(ctx, declaration, definition)
	if err != nil {
		return e.fail(ctx, declaration, inventory, phase, err)
	}
	if e.MaxTargets > 0 && len(desiredObjects) > e.MaxTargets {
		return e.fail(ctx, declaration, inventory, "ExpansionLimitExceeded", fmt.Errorf("expanded to %d resources, limit is %d", len(desiredObjects), e.MaxTargets))
	}
	if len(desiredObjects) == 0 && len(inventory.Items) == 0 {
		inventory.ObservedDeclarationResourceVersion = declaration.ResourceVersion
		inventory.ObservedRevision = inventoryRevision(nil)
		inventory.Phase = "NoTargetsMatched"
		inventory.Message = ""
		if err := e.InventoryStore.Save(ctx, declaration, inventory); err != nil {
			return Result{}, err
		}
		return Result{Inventory: inventory}, nil
	}
	if !slices.Contains(declaration.Finalizers, e.Mode.CleanupFinalizer) {
		base := declaration.DeepCopy()
		declaration.Finalizers = append(declaration.Finalizers, e.Mode.CleanupFinalizer)
		if err := e.DeclarationClient.Patch(ctx, declaration, client.MergeFrom(base)); err != nil {
			return Result{}, err
		}
		return Result{Requeue: true}, nil
	}

	desired := make([]InventoryItem, 0, len(desiredObjects))
	for _, desiredObject := range desiredObjects {
		item, err := e.apply(ctx, declaration, desiredObject)
		if err != nil {
			return e.fail(ctx, declaration, inventory, "ApplyFailed", err)
		}
		desired = append(desired, item)
	}
	if err := e.prune(ctx, declaration, inventory.Items, desired, definition.Config.Apply.DeletionPolicy); err != nil {
		return e.fail(ctx, declaration, inventory, "PruneFailed", err)
	}
	newInventory := Inventory{ObservedDeclarationResourceVersion: declaration.ResourceVersion, Phase: "Applied", Items: desired}
	newInventory.ObservedRevision = inventoryRevision(desired)
	if err := e.InventoryStore.Save(ctx, declaration, newInventory); err != nil {
		return Result{}, err
	}
	return Result{Inventory: newInventory}, nil
}

func (e *Engine) expand(ctx context.Context, declaration *corev1.Secret, definition Definition) ([]desiredObject, string, error) {
	var selectedNamespaces []NamespaceIdentity
	var namespaceSnapshot []NamespaceIdentity

	desired := make([]desiredObject, 0, len(definition.Objects))
	identities := map[string]struct{}{}
	for index, object := range definition.Objects {
		mapping, err := e.TargetMapper.RESTMapping(object.GroupVersionKind().GroupKind(), object.GroupVersionKind().Version)
		if err != nil {
			return nil, "DiscoveryFailed", fmt.Errorf("manifest %d: %w", index, err)
		}
		namespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace
		if !namespaced {
			if object.GetNamespace() != "" {
				return nil, "InvalidDefinition", fmt.Errorf("manifest %d: cluster-scoped resource must not set metadata.namespace", index)
			}
			if !e.Mode.AllowClusterScoped || !definition.Config.Security.AllowClusterScoped {
				return nil, "TargetMustBeNamespaced", fmt.Errorf("manifest %d: cluster-scoped target is not allowed", index)
			}
			desired = append(desired, desiredObject{object: object, mapping: mapping, config: definition.Config})
			continue
		}

		if definition.Config.Targets.Namespaces != nil {
			if object.GetNamespace() == "" {
				if selectedNamespaces == nil {
					selectedNamespaces, err = e.resolveTargets(ctx, declaration, definition.Config)
					if err != nil {
						return nil, "TargetsResolutionFailed", err
					}
				}
				for _, target := range selectedNamespaces {
					desired = append(desired, desiredObject{object: object, mapping: mapping, target: target, config: definition.Config})
				}
				continue
			}
		} else if object.GetNamespace() == "" {
			return nil, "InvalidDefinition", fmt.Errorf("manifest %d: namespaced resource requires metadata.namespace when targets.namespaces is not configured", index)
		}
		if object.GetNamespace() != "" {
			if namespaceSnapshot == nil {
				namespaceSnapshot, err = e.Namespaces.List(ctx)
				if err != nil {
					return nil, "TargetsResolutionFailed", err
				}
			}
			target, ok := namespaceByName(namespaceSnapshot, object.GetNamespace())
			if !ok {
				return nil, "TargetNamespaceNotFound", fmt.Errorf("manifest %d: namespace %q is not active or does not exist", index, object.GetNamespace())
			}
			desired = append(desired, desiredObject{object: object, mapping: mapping, target: target, config: definition.Config})
		}
	}
	for _, item := range desired {
		key := item.mapping.Resource.String() + "/" + item.target.Name + "/" + item.object.GetName()
		if _, exists := identities[key]; exists {
			return nil, "InvalidDefinition", fmt.Errorf("duplicate desired resource %s", key)
		}
		identities[key] = struct{}{}
		if err := e.Policy.Validate(ctx, declaration, item.mapping, item.target, item.object.DeepCopy(), definition.Config.Apply); err != nil {
			return nil, "PolicyDenied", err
		}
	}
	return desired, "", nil
}

func namespaceByName(namespaces []NamespaceIdentity, name string) (NamespaceIdentity, bool) {
	for _, namespace := range namespaces {
		if namespace.Name == name && namespace.Phase == corev1.NamespaceActive {
			return namespace, true
		}
	}
	return NamespaceIdentity{}, false
}

func (e *Engine) resolveTargets(ctx context.Context, declaration *corev1.Secret, config Config) ([]NamespaceIdentity, error) {
	namespaces, err := e.Namespaces.List(ctx)
	if err != nil {
		return nil, err
	}
	var targets []NamespaceIdentity
	for _, namespace := range namespaces {
		if namespace.Phase != corev1.NamespaceActive || (!config.IncludeDefinitionNamespace && namespace.Name == declaration.Namespace) {
			continue
		}
		if !Matches(*config.Targets.Namespaces, namespace.Name) || !labelsMatch(config.RequireTargetLabels, namespace.Labels) {
			continue
		}
		targets = append(targets, namespace)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
	return targets, nil
}

func (e *Engine) apply(ctx context.Context, declaration *corev1.Secret, desired desiredObject) (InventoryItem, error) {
	object := desired.object.DeepCopy()
	if desired.mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		object.SetNamespace(desired.target.Name)
	}
	revision, err := objectRevision(object, desired.config.Apply, desired.target)
	if err != nil {
		return InventoryItem{}, err
	}
	labels := object.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[e.Mode.OwnershipPrefix+"managed"] = "true"
	labels[e.Mode.OwnershipPrefix+"definition-name"] = declaration.Name
	object.SetLabels(labels)
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[e.Mode.OwnershipPrefix+"definition-namespace"] = declaration.Namespace
	annotations[e.Mode.OwnershipPrefix+"definition-uid"] = string(declaration.UID)
	annotations[e.Mode.OwnershipPrefix+"revision"] = revision
	if desired.target.UID != "" {
		annotations[e.Mode.OwnershipPrefix+"target-namespace-uid"] = string(desired.target.UID)
	}
	object.SetAnnotations(annotations)

	resource := e.TargetClient.Resource(desired.mapping.Resource)
	var targetResource dynamic.ResourceInterface = resource
	if object.GetNamespace() != "" {
		targetResource = resource.Namespace(object.GetNamespace())
	}
	existing, err := targetResource.Get(ctx, object.GetName(), metav1.GetOptions{})
	if err == nil {
		owner := existing.GetAnnotations()[e.Mode.OwnershipPrefix+"definition-uid"]
		if owner != string(declaration.UID) && !desired.config.Apply.AdoptExisting {
			return InventoryItem{}, fmt.Errorf("ownership conflict for %s/%s", object.GetNamespace(), object.GetName())
		}
	} else if !apierrors.IsNotFound(err) {
		return InventoryItem{}, err
	}
	data, err := json.Marshal(object.Object)
	if err != nil {
		return InventoryItem{}, err
	}
	force := desired.config.Apply.ConflictPolicy == "Force"
	applied, err := targetResource.Patch(ctx, object.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: e.Mode.FieldManagerPrefix + shortHash(string(declaration.UID)), Force: &force,
	})
	if err != nil {
		return InventoryItem{}, err
	}
	return InventoryItem{Group: desired.mapping.Resource.Group, Version: desired.mapping.Resource.Version, Resource: desired.mapping.Resource.Resource, Namespace: object.GetNamespace(), NamespaceUID: desired.target.UID, Name: object.GetName(), UID: applied.GetUID()}, nil
}

func (e *Engine) prune(ctx context.Context, declaration *corev1.Secret, previous, desired []InventoryItem, policy string) error {
	desiredSet := map[string]struct{}{}
	for _, item := range desired {
		desiredSet[itemKey(item)] = struct{}{}
	}
	for _, item := range previous {
		if _, ok := desiredSet[itemKey(item)]; ok {
			continue
		}
		if policy == "Orphan" {
			continue
		}
		if err := e.deleteOwned(ctx, declaration, item); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) cleanup(ctx context.Context, declaration *corev1.Secret, inventory Inventory) (Result, error) {
	for _, item := range inventory.Items {
		if err := e.deleteOwned(ctx, declaration, item); err != nil {
			return Result{}, err
		}
	}
	if err := e.InventoryStore.Delete(ctx, declaration); err != nil {
		return Result{}, err
	}
	current := &corev1.Secret{}
	if err := e.DeclarationClient.Get(ctx, client.ObjectKeyFromObject(declaration), current); err != nil {
		return Result{}, client.IgnoreNotFound(err)
	}
	base := current.DeepCopy()
	current.Finalizers = slices.DeleteFunc(current.Finalizers, func(value string) bool { return value == e.Mode.CleanupFinalizer })
	if err := e.DeclarationClient.Patch(ctx, current, client.MergeFrom(base)); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}

func (e *Engine) deleteOwned(ctx context.Context, declaration *corev1.Secret, item InventoryItem) error {
	resource := e.TargetClient.Resource(schema.GroupVersionResource{Group: item.Group, Version: item.Version, Resource: item.Resource})
	var target dynamic.ResourceInterface = resource
	if item.Namespace != "" {
		target = resource.Namespace(item.Namespace)
	}
	existing, err := target.Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.GetUID() != item.UID || existing.GetAnnotations()[e.Mode.OwnershipPrefix+"definition-uid"] != string(declaration.UID) {
		return fmt.Errorf("ownership lost for %s/%s", item.Namespace, item.Name)
	}
	uid := existing.GetUID()
	return target.Delete(ctx, item.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
}

func (e *Engine) fail(ctx context.Context, declaration *corev1.Secret, inventory Inventory, phase string, cause error) (Result, error) {
	inventory.Phase = phase
	inventory.Message = cause.Error()
	if err := e.InventoryStore.Save(ctx, declaration, inventory); err != nil {
		return Result{}, err
	}
	return Result{Inventory: inventory}, cause
}

func labelsMatch(required, labels map[string]string) bool {
	for key, value := range required {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func itemKey(item InventoryItem) string {
	return item.Group + "/" + item.Version + "/" + item.Resource + "/" + item.Namespace + "/" + item.Name + "/" + string(item.NamespaceUID)
}

func objectRevision(object *unstructured.Unstructured, policy ApplyPolicy, target NamespaceIdentity) (string, error) {
	data, err := json.Marshal(struct {
		Object any
		Policy ApplyPolicy
		Target NamespaceIdentity
	}{object.Object, policy, target})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func inventoryRevision(items []InventoryItem) string {
	data, _ := json.Marshal(items)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
