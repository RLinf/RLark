package sshdialer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"golang.org/x/crypto/ssh"
)

func dialSSH(ctx context.Context, sshAddr, certPEM, keyPEM string, cfg Config) (*ssh.Client, error) {
	user, address := parseSSHAddr(sshAddr, cfg.SSHUser)
	signer, err := ssh.ParsePrivateKey([]byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}

	var auth ssh.AuthMethod
	if certPEM != "" {
		certificate, err := cert.DecodeSSHCertificateFromPEM([]byte(certPEM))
		if err != nil {
			return nil, fmt.Errorf("parse ssh cert: %w", err)
		}
		certSigner, err := ssh.NewCertSigner(certificate, signer)
		if err != nil {
			return nil, fmt.Errorf("new cert signer: %w", err)
		}
		auth = ssh.PublicKeys(certSigner)
	} else {
		auth = ssh.PublicKeys(signer)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: cfg.HostKeyCallback,
		Timeout:         cfg.SSHTimeout,
	}
	dialer := &net.Dialer{Timeout: cfg.SSHTimeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial ssh server %s: %w", address, err)
	}
	handshakeDeadline := time.Now().Add(cfg.SSHTimeout)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(handshakeDeadline) {
		handshakeDeadline = deadline
	}
	if err := conn.SetDeadline(handshakeDeadline); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("set SSH handshake deadline: %w", err)
	}
	handshakeDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-handshakeDone:
		}
	}()

	c, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	close(handshakeDone)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", address, err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("clear SSH handshake deadline: %w", err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func parseSSHAddr(addr, defaultUser string) (string, string) {
	user, hostPort, ok := strings.Cut(addr, "@")
	if ok {
		return user, hostPort
	}
	return defaultUser, addr
}

func isSSHTransportError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	return strings.Contains(err.Error(), "ssh: tcp transport closed")
}

func isSSHChannelTransportError(err error) bool {
	return !errors.Is(err, io.EOF) && isSSHTransportError(err)
}

func (d *DialerManager) dialSSHWithMergedCtx(ctx context.Context, info DomainInfo) (*ssh.Client, error) {
	dialCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-d.ctx.Done():
			cancel()
		case <-dialCtx.Done():
		}
	}()
	return dialSSH(dialCtx, info.SSHAddress, info.Certificate, info.PrivateKey, d.cfg)
}
