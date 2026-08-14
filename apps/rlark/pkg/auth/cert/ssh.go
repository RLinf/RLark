package cert

import (
	"encoding/json"

	gossh "golang.org/x/crypto/ssh"
)

const metaSSHExtKey = "x-rlark-meta@rlark.io"

// SetSSHCertMeta sets the sSHCertMeta.
func SetSSHCertMeta(template *gossh.Certificate, meta map[string]string) {
	if template == nil {
		return
	}
	if template.Extensions == nil {
		template.Extensions = make(map[string]string)
	}
	metaBytes, _ := json.Marshal(meta)
	template.Extensions[metaSSHExtKey] = string(metaBytes)
}

// GetSSHCertMeta returns the sSHCertMeta.
func GetSSHCertMeta(cert *gossh.Certificate) (map[string]string, bool) {
	if cert == nil || cert.Extensions == nil {
		return nil, false
	}
	metaStr, ok := cert.Extensions[metaSSHExtKey]
	if !ok {
		return nil, false
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
		return nil, false
	}
	return meta, true
}
