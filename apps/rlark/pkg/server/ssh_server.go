package server

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	gossh "golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/server/reverseproxy"
)

func (s *Server) runSSHServer(ctx context.Context) error {
	logger := log.FromContext(ctx)
	server, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf(":%d", s.config.SSHPort)),
		wish.WithHostKeyPEM(s.tlsCA.KeyPEM),
		s.channelOption,
		ssh.PublicKeyAuth(s.sshPublicKeyAuth()),
		wish.WithMiddleware(s.sshMiddleware()),
	)
	if err != nil {
		return fmt.Errorf("failed to create SSH server: %w", err)
	}

	go func() {
		<-ctx.Done()
		logger.Info("Shutting down SSH server", "port", s.config.SSHPort)
		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error(nil, "SSH server shutdown error", "err", err)
		}
	}()

	logger.Info("Starting SSH server", "port", s.config.SSHPort)
	return server.ListenAndServe()
}

func (s *Server) sshMiddleware() wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(sess ssh.Session) {
			if len(sess.Command()) > 0 {
				_, _ = fmt.Fprintln(sess, "Disallowed command")
			} else {
				pty, sizeCh, isPty := sess.Pty()
				if !isPty {
					_, _ = fmt.Fprintf(sess, "Welcome to RLark, @%v!\n", sess.User())
				} else {
					meta, _ := cert.GetSSHCertMeta(&gossh.Certificate{Permissions: *sess.Permissions().Permissions})
					js := NewJumperStub(s, sess, sess.User(), meta)
					if js.JumperAvailable() {
						js.Serve(sess.Context(), pty, sizeCh)
					} else {
						_, _ = fmt.Fprintln(sess, "PTY allocation request failed")
					}
				}
			}
		}
	}
}

type wrapSSHMetadata struct {
	ssh.Context
}

// SessionID is an exported method.
func (m wrapSSHMetadata) SessionID() []byte {
	return []byte(m.Context.SessionID())
}

// ClientVersion is an exported method.
func (m wrapSSHMetadata) ClientVersion() []byte {
	return []byte(m.Context.ClientVersion())
}

// ServerVersion is an exported method.
func (m wrapSSHMetadata) ServerVersion() []byte {
	return []byte(m.Context.ServerVersion())
}

func (s *Server) sshPublicKeyAuth() ssh.PublicKeyHandler {
	logger := log.GetLogger()
	// 从 s.caRootPEM 加载受信任的 CA 公钥
	var caKeys []gossh.PublicKey
	for _, ca := range s.ca {
		if ca.Key != nil {
			key, err := gossh.NewPublicKey(ca.Key.Public())
			if err == nil {
				caKeys = append(caKeys, key)
			}
		}
	}
	if len(caKeys) == 0 {
		logger.Error(nil, "No SSH CA keys found in caRootPEM, all certificate auth will be rejected")
	}

	cc := &gossh.CertChecker{
		IsUserAuthority: func(auth gossh.PublicKey) bool {
			logger.V(1).Info("Checking if public key is a trusted CA", "key", auth)
			a := auth.Marshal()
			for _, caKey := range caKeys {
				b := caKey.Marshal()
				if len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1 {
					logger.V(1).Info("Public key is a trusted CA", "key", auth)
					return true
				}
			}
			logger.V(1).Info("Public key is NOT a trusted CA", "key", auth)
			return false
		},
		IsRevoked: func(cert *gossh.Certificate) bool {
			logger.V(1).Info("Checking if certificate is revoked", "serial", cert.Serial, "keyID", cert.KeyId)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ret := s.checkCertRevoked(ctx, "ssh", fmt.Sprint(cert.Serial), cert.KeyId)
			logger.V(1).Info("Certificate revocation check result", "serial", cert.Serial, "keyID", cert.KeyId, "revoked", ret)
			return ret
		},
		UserKeyFallback: func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			logger.V(1).Info("UserKeyFallback: checking user key store for public key", "user", conn.User())
			keyID, err := s.authenticateUserKey(conn.User(), key)
			if err != nil {
				return nil, fmt.Errorf("public key authentication failed for user %s: %w", conn.User(), err)
			}
			if keyID == "" {
				return nil, fmt.Errorf("public key not found for user %s", conn.User())
			}
			_, meta, _ := s.parseSignRequest(&SignRequest{
				Role:     "ssh-guest",
				ClientID: conn.User(),
				KeyID:    keyID,
			})
			sshCert := &gossh.Certificate{}
			cert.SetSSHCertMeta(sshCert, meta)
			return &sshCert.Permissions, nil
		},
	}

	return func(ctx ssh.Context, key ssh.PublicKey) bool {
		perm, err := cc.Authenticate(wrapSSHMetadata{ctx}, key)
		if err != nil || perm == nil {
			metrics.IncSSHConnection(ctx.User(), "auth_failed")
			logger.Error(nil, "SSH public key authentication failed", "user", ctx.User(), "err", err)
			return false
		}
		metrics.IncSSHConnection(ctx.User(), "auth_ok")
		ctx.Permissions().Permissions = perm
		logger.V(1).Info("SSH public key authenticated", "permissions", perm.Extensions)
		return true
	}
}

func (s *Server) channelOption(srv *ssh.Server) error {
	if srv.ChannelHandlers == nil {
		srv.ChannelHandlers = make(map[string]ssh.ChannelHandler)
	}
	srv.ChannelHandlers["direct-tcpip"] = s.handleSSHChannel
	srv.ChannelHandlers["session"] = ssh.DefaultSessionHandler
	return nil
}

func (s *Server) rejectSSHChannel(newChan gossh.NewChannel, message string) {
	logger := log.GetLogger()
	if err := newChan.Reject(gossh.Prohibited, message); err != nil {
		logger.Error(nil, "Cannot reject channel", "err", err)
	}
}

func (s *Server) handleSSHChannel(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	logger := log.GetLogger()
	logger.V(1).Info("Handle direct-tcpip", "user", ctx.User(), "sessionID", ctx.SessionID())

	permissions := ctx.Permissions()
	if permissions == nil || permissions.Permissions == nil {
		logger.Error(nil, "No permissions found for session", "sessionID", ctx.SessionID())
		s.rejectSSHChannel(newChan, "No permissions found")
		return
	}
	certMeta, ok := cert.GetSSHCertMeta(&gossh.Certificate{Permissions: *permissions.Permissions})
	if !ok {
		logger.Error(nil, "No metadata found for session", "sessionID", ctx.SessionID())
		s.rejectSSHChannel(newChan, "No metadata found in certificate")
		return
	}

	var payload apis.SSHDirectPayload
	err := gossh.Unmarshal(newChan.ExtraData(), &payload)
	if err != nil {
		logger.Error(nil, "Cannot accept extra data", "sessionID", ctx.SessionID(), "err", err)
		s.rejectSSHChannel(newChan, fmt.Sprintf("Invalid channel payload: %v", err))
		return
	}

	address := net.JoinHostPort(payload.Host, fmt.Sprint(payload.Port))
	dialer, target, err := s.GetDial(ctx, "ssh", address, certMeta)
	if err != nil {
		logger.Error(nil, "Cannot get dialer", "sessionID", ctx.SessionID(), "err", err)
		s.rejectSSHChannel(newChan, fmt.Sprintf("Cannot get dialer: %v", err))
		return
	}

	ch, _, err := newChan.Accept()
	if err != nil {
		logger.Error(nil, "Cannot accept channel", "sessionID", ctx.SessionID(), "err", err)
		s.rejectSSHChannel(newChan, fmt.Sprintf("Failed to accept channel: %v", err))
		return
	}
	defer func() { _ = ch.Close() }()

	c, err := dialer(ctx, "tcp", target)
	if err != nil {
		logger.Error(nil, "Cannot dial", "sessionID", ctx.SessionID(), "err", err)
		s.rejectSSHChannel(newChan, fmt.Sprintf("Failed to connect to target: %v", err))
		return
	}
	reverseproxy.PipeConnections(ch, c)
}

func (s *Server) authenticateUserKey(username string, key gossh.PublicKey) (string, error) {
	if s.userKeyStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		keys, err := s.userKeyStore.GetSSHUserKeysByUser(ctx, username)
		if err != nil {
			return "", err
		}
		for _, k := range keys {
			pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k.PublicKey))
			if err == nil && ssh.KeysEqual(key, pubKey) {
				return fmt.Sprint(k.ID), nil
			}
		}
		return "", nil
	}

	return s.authenticateUserKeyFromSecret(username, key)
}

func (s *Server) authenticateUserKeyFromSecret(username string, key gossh.PublicKey) (string, error) {
	if s.kubeClient == nil {
		return "", fmt.Errorf("kubernetes client not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	secret, err := s.kubeClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, common.SSHUserKeySecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	raw, ok := secret.Data[username]
	if !ok {
		return "", nil
	}

	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err == nil && ssh.KeysEqual(key, pubKey) {
			return fmt.Sprintf("%s-%d", username, i), nil
		}
	}

	return "", nil
}
