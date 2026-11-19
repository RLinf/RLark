package api

import (
	"bytes"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DecodeKubernetesDaemonSetCarrierParameters strictly decodes the DaemonSet
// carrier's parameters and fills in MaxUnavailable. Unlike the replicated
// carriers, absent parameters are valid: every field has a default. The Job
// Controller calls it to reject a bad per-node Task before dispatch, and the
// Agent to render the DaemonSet.
func DecodeKubernetesDaemonSetCarrierParameters(parameters apiextensionsv1.JSON) (KubernetesDaemonSetCarrierParametersV1, error) {
	result := KubernetesDaemonSetCarrierParametersV1{}
	if len(bytes.TrimSpace(parameters.Raw)) != 0 {
		decoded, err := DecodeParametersStrict[KubernetesDaemonSetCarrierParametersV1](parameters)
		if err != nil {
			return result, err
		}
		result = decoded
	}
	if result.Canary != nil && *result.Canary < 1 {
		return result, fmt.Errorf("canary must be a positive integer")
	}
	if result.MaxUnavailable != nil {
		value, err := intstr.GetScaledValueFromIntOrPercent(result.MaxUnavailable, 100, true)
		if err != nil || value < 1 || (result.MaxUnavailable.Type == intstr.String && value > 100) {
			return result, fmt.Errorf("maxUnavailable must be a positive integer or a percentage from 1%% to 100%%")
		}
	}
	return result, nil
}
