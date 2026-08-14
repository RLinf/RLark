package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	gocache "github.com/patrickmn/go-cache"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// Constants used by the package.
const (
	ContextKeyCertMeta = "certMeta"
)

// GetCertMetaFromContext returns the certMetaFromContext.
func GetCertMetaFromContext(ctx *gin.Context) map[string]string {
	if meta, exists := ctx.Get(ContextKeyCertMeta); exists {
		if certMeta, ok := meta.(map[string]string); ok {
			return certMeta
		}
	}
	return map[string]string{}
}

// SignRequest represents a request.
type SignRequest struct {
	Role     string `json:"role"`                // 证书角色，例如 "agent" 等
	ClientID string `json:"client_id,omitempty"` // 可选的客户端 ID
	DomainID string `json:"domain_id,omitempty"` // 可选的域 ID
	KeyID    string `json:"key_id,omitempty"`    // 可选的密钥 ID
}

// SignResponse represents a response.
type SignResponse struct {
	CertType     string `json:"cert_type"` // 证书类型，例如 "x509" 或 "ssh"
	SerialNumber string `json:"serial_number,omitempty"`
	SubjectKeyID string `json:"subject_key_id,omitempty"`
	CertPEM      string `json:"cert_pem"` // PEM 编码的证书
	KeyPEM       string `json:"key_pem"`  // PEM 编码的私钥
}

// RevokeCertRequest represents a request.
type RevokeCertRequest struct {
	CertType     string `json:"cert_type"`                // 证书类型，例如 "x509" 或 "ssh"
	SerialNumber string `json:"serial_number,omitempty"`  // 可选的证书序列号
	SubjectKeyID string `json:"subject_key_id,omitempty"` // 可选的证书主题密钥 ID
	Reason       string `json:"reason,omitempty"`         // 可选的吊销原因
}

func (s *Server) parseSignRequest(req *SignRequest) (string, map[string]string, error) {
	switch req.Role {
	case "admin":
		return "x509", map[string]string{
			apis.MetaCertRole:                "admin",
			apis.MetaPermissionAdmin:         "true", // TODO
			apis.MetaKubernetesImpersonation: "-",
		}, nil

	case "peer":
		return "x509", map[string]string{
			apis.MetaCertRole:              "peer",
			apis.MetaRemoteDialerPeerID:    s.dialerFactory.GetPeerID(),
			apis.MetaRemoteDialerPeerToken: s.dialerFactory.GetPeerToken(),
		}, nil

	case "agent":
		if req.ClientID == "" {
			return "", nil, fmt.Errorf("client_id is required for agent role")
		}
		namespace := apis.RLarkAgentNamespacePrefix + req.ClientID
		impersonation := "system:serviceaccount:" + namespace + ":" + apis.RLarkAgentServiceAccountName
		return "x509", map[string]string{
			apis.MetaCertRole:                "agent",
			apis.MetaAgentID:                 req.ClientID,
			apis.MetaNamespace:               namespace,
			apis.MetaRemoteDialerClientID:    req.ClientID,
			apis.MetaKubernetesImpersonation: impersonation,
		}, nil

	case "domain":
		if req.ClientID == "" || req.DomainID == "" {
			return "", nil, fmt.Errorf("client_id and domain_id are required for domain role")
		}
		return "ssh", map[string]string{
			apis.MetaCertRole: "domain",
			apis.MetaAgentID:  req.ClientID,
			apis.MetaDomainID: req.DomainID,
		}, nil

	case "ssh-guest":
		if req.ClientID == "" || req.KeyID == "" {
			return "", nil, fmt.Errorf("client_id and key_id are required for ssh-guest role")
		}
		return "ssh", map[string]string{
			apis.MetaCertRole:  "ssh-guest",
			apis.MetaUserID:    req.ClientID,
			apis.MetaUserKeyID: req.KeyID,
		}, nil

	default:
		return "", nil, fmt.Errorf("unsupported role: %s", req.Role)
	}
}

func (s *Server) handleSignCertificate(ctx *gin.Context) {
	if !auth.PermissionChecker.IsAdmin(GetCertMetaFromContext(ctx)) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "admin permission required"})
		return
	}

	if len(s.ca) == 0 {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "no CA available"})
		return
	}

	var req SignRequest
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	signType, meta, err := s.parseSignRequest(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cert, err := cert.Sign(&s.ca[0], signType, meta)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "sign certificate failed"})
		return
	}
	resp := SignResponse{
		CertType: signType,
		CertPEM:  string(cert.CertPEM),
		KeyPEM:   string(cert.KeyPEM),
	}
	if cert.Cert != nil {
		resp.SerialNumber = cert.Cert.SerialNumber.String()
		resp.SubjectKeyID = fmt.Sprintf("%x", cert.Cert.SubjectKeyId)
	} else if cert.SSHCert != nil {
		resp.SerialNumber = fmt.Sprint(cert.SSHCert.Serial)
		resp.SubjectKeyID = cert.SSHCert.KeyId
	}
	ctx.JSON(http.StatusOK, resp)
}

func (s *Server) checkCertRevoked(ctx context.Context, certType, serialNumber, subjectKeyID string) bool {
	cacheKey := fmt.Sprintf("%s:%s:%s", certType, serialNumber, subjectKeyID)
	if cached, found := s.rcCache.Get(cacheKey); found {
		ret, _ := cached.(bool)
		return ret
	}
	if s.rcStore == nil {
		return false
	}
	revoked, _ := s.rcStore.IsCertificateRevoked(ctx, certType, serialNumber, subjectKeyID)
	s.rcCache.Set(cacheKey, revoked, gocache.DefaultExpiration)
	return revoked
}

func (s *Server) handleCertCheck(ctx *gin.Context) {
	var meta map[string]string
	if len(ctx.Request.TLS.PeerCertificates) > 0 {
		clientCert := ctx.Request.TLS.PeerCertificates[0]
		subjectKeyID := fmt.Sprintf("%x", clientCert.SubjectKeyId)
		if s.checkCertRevoked(ctx, "x509", clientCert.SerialNumber.String(), subjectKeyID) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "client certificate revoked"})
			ctx.Abort()
			return
		}
		meta, _ = cert.GetX509CertMeta(clientCert)
	} else {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "client certificate required"})
		ctx.Abort()
		return
	}

	if meta != nil {
		ctx.Set(ContextKeyCertMeta, meta)
	}
	ctx.Next()
}

func (s *Server) handleRevokeCertificate(ctx *gin.Context) {
	if !auth.PermissionChecker.IsAdmin(GetCertMetaFromContext(ctx)) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "admin permission required"})
		return
	}

	var req RevokeCertRequest
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if s.rcStore != nil {
		model := &db.RevokedCertificateModel{
			CertType:         req.CertType,
			SerialNumber:     req.SerialNumber,
			SubjectKeyID:     req.SubjectKeyID,
			RevocationReason: req.Reason,
			RevokedAt:        time.Now(),
		}
		if err := s.rcStore.AddRevokedCertificate(ctx, model); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke certificate"})
			return
		}
	} else {
		logger := log.GetLogger()
		logger.Error(nil, "RevokedCertificateStore is not configured, only in-memory cache will be used for revocation check")
	}
	key := fmt.Sprintf("%s:%s:%s", req.CertType, req.SerialNumber, req.SubjectKeyID)
	s.rcCache.Set(key, true, gocache.DefaultExpiration)
}
