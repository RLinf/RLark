package utils

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ParentUIDAnnotation     = "rlark.io/parent-uid"
	ChildTemplateAnnotation = "rlark.io/child-template"
)

// ClassifyChild accepts explicit ownership and the label-only format emitted by
// older controllers. Any contradictory metadata is a conflict.
func ClassifyChild(obj metav1.Object, parentUID types.UID, parentAPIVersion, parentKind, parentLabel, parentName, templateName string) (owned, legacy bool, err error) {
	annotations := obj.GetAnnotations()
	uid, hasUID := annotations[ParentUIDAnnotation]
	template, hasTemplate := annotations[ChildTemplateAnnotation]
	owner := metav1.GetControllerOf(obj)

	ownerMatches := owner != nil && owner.UID == parentUID && owner.APIVersion == parentAPIVersion &&
		owner.Kind == parentKind && owner.Name == parentName
	if ownerMatches && !hasUID && !hasTemplate {
		return true, true, nil
	}

	if hasUID || hasTemplate || owner != nil {
		if !hasUID || uid != string(parentUID) || !hasTemplate || template != templateName ||
			!ownerMatches {
			return false, false, fmt.Errorf("child %s has conflicting ownership metadata", obj.GetName())
		}
		return true, false, nil
	}

	if obj.GetLabels()[parentLabel] == parentName {
		return true, true, nil
	}
	return false, false, fmt.Errorf("child %s already exists and is not owned by %s", obj.GetName(), parentName)
}

// ClassifyRemovedChild accepts authoritative ownership directly. Label-only
// legacy children additionally require template identity from the old parent
// status/spec snapshot; child names are not assumed to be reversible.
func ClassifyRemovedChild(obj metav1.Object, parentUID types.UID, parentAPIVersion, parentKind, parentLabel, parentName string, oldTemplates []string) (owned, legacy bool) {
	annotations := obj.GetAnnotations()
	uid, hasUID := annotations[ParentUIDAnnotation]
	_, hasTemplate := annotations[ChildTemplateAnnotation]
	owner := metav1.GetControllerOf(obj)
	ownerMatches := owner != nil && owner.UID == parentUID && owner.APIVersion == parentAPIVersion &&
		owner.Kind == parentKind && owner.Name == parentName

	if owner != nil && !ownerMatches || hasUID && uid != string(parentUID) {
		return false, false
	}
	if ownerMatches || hasUID {
		return true, !hasUID || !hasTemplate
	}
	if hasTemplate {
		return false, false
	}
	if obj.GetLabels()[parentLabel] != parentName {
		return false, false
	}
	for _, candidate := range oldTemplates {
		if ChildName(parentName, candidate) == obj.GetName() {
			return true, true
		}
	}
	return false, false
}

func MergeAnnotations(existing, owned map[string]string) map[string]string {
	merged := make(map[string]string, len(existing)+len(owned))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range owned {
		if value == "" {
			delete(merged, key)
		} else {
			merged[key] = value
		}
	}
	return merged
}
