package api

import (
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestDecodeParametersStrict(t *testing.T) {
	t.Parallel()

	parameters, err := DecodeParametersStrict[RayExecutionParametersV1](apiextensionsv1.JSON{Raw: []byte(`{"role":"head","prepare":{"inline":"echo ready"}}`)})
	if err != nil {
		t.Fatalf("DecodeParametersStrict() error = %v", err)
	}
	if parameters.Role != RayRoleHead || parameters.Prepare == nil || parameters.Prepare.Inline != "echo ready" {
		t.Fatalf("DecodeParametersStrict() = %#v", parameters)
	}
}

func TestDecodeParametersStrictRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":          "",
		"unknown field":  `{"role":"head","unknown":true}`,
		"trailing value": `{"role":"head"} {"role":"worker"}`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeParametersStrict[RayExecutionParametersV1](apiextensionsv1.JSON{Raw: []byte(input)}); err == nil {
				t.Fatal("DecodeParametersStrict() error = nil")
			}
		})
	}
}
