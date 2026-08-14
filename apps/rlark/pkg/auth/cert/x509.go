package cert

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
)

var metaX509OID = []int{1, 3, 6, 1, 4, 1, 99999, 32}

// SetX509CertMeta sets the x509CertMeta.
func SetX509CertMeta(template *x509.Certificate, meta map[string]string) {
	if template == nil {
		return
	}
	metaBytes, _ := json.Marshal(meta)
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       metaX509OID,
		Critical: false,
		Value:    metaBytes,
	})
}

// GetX509CertMeta returns the x509CertMeta.
func GetX509CertMeta(cert *x509.Certificate) (map[string]string, bool) {
	if cert == nil {
		return nil, false
	}
	// 创建证书时，会将 Meta 存于 ExtraExtensions 中，但是证书经过 TLS 握手后，
	// Meta 可能会出现在 Extensions 中，因此需要同时检查两个字段
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(metaX509OID) {
			var meta map[string]string
			if err := json.Unmarshal(ext.Value, &meta); err != nil {
				return nil, false
			}
			return meta, true
		}
	}
	for _, ext := range cert.ExtraExtensions {
		if ext.Id.Equal(metaX509OID) {
			var meta map[string]string
			if err := json.Unmarshal(ext.Value, &meta); err != nil {
				return nil, false
			}
			return meta, true
		}
	}
	return nil, false
}
