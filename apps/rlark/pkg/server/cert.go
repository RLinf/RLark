package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultTLSSecretName      = "rlark-tls"
	defaultClientCASecretName = "rlark-client-ca"
)

func (s *Server) loadTLSCA(ctx context.Context) error {
	logger := log.FromContext(ctx)
	// 读取 Kubernetes Secret 中保存的 TLS CA 证书和私钥，如果不存在则跳过（因为 TLS 证书可能是由外部 CA 签发的，不需要服务器自己管理 CA）。

	secret, err := s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Get(ctx, common.TLSCASecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			if s.config.AutoSignTLSCACert {
				logger.Info("TLS CA secret not found, creating a self-signed TLS CA certificate", "namespace", s.config.KubeClientConfig.DefaultNamespace(), "name", common.TLSCASecretName)
				return s.createAndStoreCA(ctx, common.TLSCASecretName, func(d *cert.Data) {
					s.tlsCA = d
				})
			}
			return nil
		}
		return err
	}
	if secret.Data == nil {
		return fmt.Errorf("TLS CA secret %s/%s has no data", s.config.KubeClientConfig.DefaultNamespace(), common.TLSCASecretName)
	}
	ca, err := cert.LoadData(secret.Data["ca.crt"], secret.Data["ca.key"])
	if err != nil {
		return err
	}

	s.tlsCA = ca
	return nil
}

func (s *Server) loadTLSConfig(ctx context.Context) error {
	logger := log.FromContext(ctx)
	// 读取 Kubernetes Secret 中保存的 TLS 证书和私钥，如果不存在则尝试使用 TLS CA 生成一个自签名的 TLS 证书并保存到 Kubernetes 中。

	secret, err := s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Get(ctx, defaultTLSSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			if s.tlsCA == nil {
				return fmt.Errorf("TLS secret %s/%s not found, please create it with the server certificate and key", s.config.KubeClientConfig.DefaultNamespace(), defaultTLSSecretName)
			}
			logger.Info("TLS secret not found, creating a self-signed TLS certificate using the CA", "namespace", s.config.KubeClientConfig.DefaultNamespace(), "name", defaultTLSSecretName)
			return s.createAndStoreTLSConfig(ctx, s.tlsCA)
		}
		return err
	}
	if secret.Data == nil {
		return fmt.Errorf("TLS secret %s/%s has no data", s.config.KubeClientConfig.DefaultNamespace(), defaultTLSSecretName)
	}
	data, err := cert.LoadData(secret.Data[v1.TLSCertKey], secret.Data[v1.TLSPrivateKeyKey])
	if err != nil {
		return fmt.Errorf("load TLS data: %w", err)
	}

	s.tls = *data
	return nil
}

func (s *Server) createAndStoreTLSConfig(ctx context.Context, ca *cert.Data) error {
	// 生成一个新的 TLS 证书，并且使用 CA 进行签名，然后保存到 Kubernetes Secret 中。

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate TLS cert key: %w", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	commonName := "rlark-server"
	if len(s.config.TLSDomains) > 0 {
		commonName = s.config.TLSDomains[0]
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"RLinf"},
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		PublicKey:   leafKey.Public(),
		DNSNames:    s.config.TLSDomains,
	}

	certPEM, err := ca.SignX509Certificate(template)
	if err != nil {
		return fmt.Errorf("sign TLS certificate: %w", err)
	}
	keyPEM, err := cert.EncodePrivateKeyToPEM(leafKey)
	if err != nil {
		return fmt.Errorf("encode TLS private key: %w", err)
	}

	data := map[string][]byte{
		v1.TLSCertKey:       certPEM,
		v1.TLSPrivateKeyKey: keyPEM,
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultTLSSecretName,
			Namespace: s.config.KubeClientConfig.DefaultNamespace(),
			Labels: map[string]string{
				"app":  "rlark",
				"type": "tls",
			},
			Annotations: map[string]string{
				"description": "TLS certificate and key for RLark server",
			},
			Finalizers: []string{"rlark.io/tls-secret-protection"},
		},
		Data: data,
		Type: v1.SecretTypeTLS,
	}
	_, err = s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create TLS secret: %w", err)
	}
	tls, err := cert.LoadData(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("load TLS data: %w", err)
	}
	s.tls = *tls
	return nil
}

func (s *Server) initCAConfigs(ctx context.Context) error {
	// 读取 Kubernetes Secret 中保存的 CA 证书和私钥，如果不存在则创建一个新的 CA 并保存到 Kubernetes 中。

	secret, err := s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Get(ctx, defaultClientCASecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// 如果 Secret 不存在，则创建一个新的 CA 并保存到 Kubernetes 中。
			return s.createAndStoreCA(ctx, defaultClientCASecretName, func(d *cert.Data) {
				s.ca = append(s.ca, *d)
			})
		}
		return err
	}
	if secret.Data == nil {
		return fmt.Errorf("CA secret %s/%s has no data", s.config.KubeClientConfig.DefaultNamespace(), defaultClientCASecretName)
	}
	ca, err := cert.LoadData(secret.Data["ca.crt"], secret.Data["ca.key"])
	if err != nil {
		return err
	}

	s.ca = append(s.ca, *ca)
	return nil
}

func (s *Server) createAndStoreCA(ctx context.Context, name string, callback func(*cert.Data)) error {
	// 生成一个新的 CA 证书和私钥，并且保存到 Kubernetes Secret 中。

	ca, err := cert.GenerateCA(cert.GenerateTemplateCA())
	if err != nil {
		return err
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.config.KubeClientConfig.DefaultNamespace(),
			Labels: map[string]string{
				"app":  "rlark",
				"type": "ca",
			},
			Annotations: map[string]string{
				"description": "CA certificate and key for RLark server",
			},
			Finalizers: []string{"rlark.io/ca-secret-protection"},
		},
		Data: map[string][]byte{
			"ca.crt": ca.CertPEM,
			"ca.key": ca.KeyPEM,
		},
		Type: v1.SecretTypeOpaque,
	}
	_, err = s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	callback(ca)
	return nil
}

func (s *Server) signAdminCert(ctx context.Context) error {
	if len(s.ca) == 0 {
		return fmt.Errorf("no CA available to sign admin certificate")
	}

	// 生成一个新的 Admin 证书，并且使用第一个 CA 进行签名。
	// 证书用于其他内部组件访问服务器的 API，具有管理员权限。
	signType, meta, err := s.parseSignRequest(&SignRequest{Role: "admin"})
	if err != nil {
		return fmt.Errorf("parse sign request: %w", err)
	}
	certData, err := cert.Sign(&s.ca[0], signType, meta)
	if err != nil {
		return fmt.Errorf("sign admin certificate: %w", err)
	}
	data := map[string][]byte{
		"client.crt": certData.CertPEM,
		"client.key": certData.KeyPEM,
	}

	secret, err := s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Get(ctx, common.AdminCertSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			secret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      common.AdminCertSecretName,
					Namespace: s.config.KubeClientConfig.DefaultNamespace(),
					Labels: map[string]string{
						"app":  "rlark",
						"type": "admin-cert",
					},
					Annotations: map[string]string{
						"description": "Admin client certificate and key for RLark server",
					},
					Finalizers: []string{"rlark.io/admin-cert-secret-protection"},
				},
				Data: data,
				Type: v1.SecretTypeOpaque,
			}
			if _, err := s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create admin cert secret: %w", err)
			}
			return nil
		}
		return fmt.Errorf("get admin cert secret: %w", err)
	}

	secret.Data = data
	if _, err := s.kubeClient.CoreV1().Secrets(s.config.KubeClientConfig.DefaultNamespace()).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update admin cert secret: %w", err)
	}
	return nil
}
