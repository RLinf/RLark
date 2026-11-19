package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
)

func declarationDigest(declaration *corev1.Secret) string {
	data, _ := json.Marshal(declaration.Data)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
