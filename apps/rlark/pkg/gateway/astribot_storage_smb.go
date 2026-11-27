package gateway

// SMB storage classes: field validation, the rclone remote path, the
// reachability probe and the browsing restriction. storage_handler.go only
// dispatches on the storage type.

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/gin-gonic/gin"
)

// smbRemotePath is the rclone path for an smb remote: "share" or "share/sub".
func smbRemotePath(stored storedStorageClass) string {
	if stored.SubPath == "" {
		return stored.Share
	}
	return stored.Share + "/" + stored.SubPath
}

// validateSMBReachable only proves the SMB port accepts connections; the
// gateway has no SMB client, so credentials are checked when the CSI driver
// mounts the share.
func validateSMBReachable(ctx context.Context, host string) error {
	ctx, cancel := context.WithTimeout(ctx, storageClassValidationTimeout)
	defer cancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, smbDialPort))
	if err != nil {
		return fmt.Errorf("connect smb server: %w", err)
	}
	return conn.Close()
}

// normalizeStorageType maps an omitted type to s3 for backward compatibility.
func normalizeStorageType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return storageTypeS3
	}
	return value
}

// smbDialPort is the SMB port probed on create/update; tests override it.
var smbDialPort = "445"

var smbHostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// cleanSMBPath trims slashes and rejects segments that could escape the share
// or break the rclone path.
func cleanSMBPath(field, value string, allowNested bool) (string, error) {
	value = strings.Trim(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")), "/")
	if value == "" {
		return "", nil
	}
	segments := strings.Split(value, "/")
	if !allowNested && len(segments) > 1 {
		return "", fmt.Errorf("%s must be a single share name", field)
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, "\x00\r\n") {
			return "", fmt.Errorf("invalid %s %q", field, value)
		}
	}
	return value, nil
}

func normalizeSMBFields(req CreateStorageClassRequest, stored *storedStorageClass) error {
	host := strings.TrimSpace(req.Host)
	host = strings.TrimPrefix(strings.TrimPrefix(host, "smb://"), "//")
	if !smbHostPattern.MatchString(host) {
		return fmt.Errorf("invalid smb host %q: use a hostname or IP without scheme, port or path", req.Host)
	}
	share, err := cleanSMBPath("share", req.Share, false)
	if err != nil {
		return err
	}
	subPath, err := cleanSMBPath("sub_path", req.SubPath, true)
	if err != nil {
		return err
	}
	for _, field := range []struct{ name, value string }{
		{"username", req.Username}, {"password", req.Password}, {"domain", req.Domain},
	} {
		if strings.ContainsAny(field.value, "\r\n") {
			return fmt.Errorf("%s must not contain line breaks", field.name)
		}
	}
	stored.Type = storageTypeSMB
	stored.Host, stored.Share, stored.SubPath = host, share, subPath
	stored.Username = strings.TrimSpace(req.Username)
	stored.Password = req.Password
	stored.Domain = strings.TrimSpace(req.Domain)
	return nil
}

// errStorageBrowsingUnsupported is returned for storage types without an
// object API in the gateway (SMB is only mounted by the CSI driver).
var errStorageBrowsingUnsupported = stderrors.New("storage class type does not support file browsing")

func respondStorageClientError(c *gin.Context, err error) {
	if stderrors.Is(err, errStorageBrowsingUnsupported) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
}

// smbSecretTemplate is the rclone smb remote config. Pass must already be in
// rclone's obscured form.
const smbSecretTemplate = `
[smb]
type = smb
host = {{ .Host }}
user = {{ .User }}
pass = {{ .Pass }}
{{- if .Domain }}
domain = {{ .Domain }}
{{- end }}
`

var smbSecretTmpl = template.Must(template.New("smb-secret").Parse(smbSecretTemplate))
