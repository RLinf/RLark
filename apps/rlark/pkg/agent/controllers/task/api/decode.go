package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// DecodeParametersStrict 将扩展参数严格解码为对应模块拥有的参数合同。
// 它拒绝未知字段、尾随 JSON 值和空参数，错误不会回显参数内容。
func DecodeParametersStrict[T any](parameters apiextensionsv1.JSON) (T, error) {
	var result T
	if len(bytes.TrimSpace(parameters.Raw)) == 0 {
		return result, fmt.Errorf("decode task extension parameters: parameters are required")
	}

	decoder := json.NewDecoder(bytes.NewReader(parameters.Raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode task extension parameters: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return result, fmt.Errorf("decode task extension parameters: multiple JSON values are not allowed")
	} else if err != io.EOF {
		return result, fmt.Errorf("decode task extension parameters: %w", err)
	}
	return result, nil
}
