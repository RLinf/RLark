package sshdialer

import (
	"errors"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultIdleTimeout              = 24 * time.Hour
	defaultCleanupInterval          = time.Minute
	defaultSSHUser                  = "root"
	defaultSSHTimeout               = 10 * time.Second
	defaultKeepaliveInterval        = 30 * time.Second
	defaultKeepaliveTimeout         = 10 * time.Second
	defaultKeepaliveDrainGrace      = 30 * time.Second
	defaultMaxConnections           = 4
	defaultMaxConnectionAge         = 24 * time.Hour
	defaultMaxChannels              = 4096
	defaultMaxChannelsPerConnection = 256
	maxReconnectBackoff             = 30 * time.Second
	initialReconnectBackoff         = time.Second
	activityUpdateInterval          = time.Second
)

const keepaliveRequest = "keepalive@openssh.com"

var (
	ErrClosed        = errors.New("ssh dialer: closed")
	ErrDomainRemoved = errors.New("ssh dialer: domain removed")
	ErrOverloaded    = errors.New("ssh dialer: channel limit reached")
)

// Config configures the SSH connection pool.
type Config struct {
	IdleTimeout              time.Duration         `json:"idleTimeout,omitempty" yaml:"idleTimeout,omitempty"`
	CleanupInterval          time.Duration         `json:"cleanupInterval,omitempty" yaml:"cleanupInterval,omitempty"`
	SSHUser                  string                `json:"sshUser,omitempty" yaml:"sshUser,omitempty"`
	SSHTimeout               time.Duration         `json:"sshTimeout,omitempty" yaml:"sshTimeout,omitempty"`
	InitialReconnectBackoff  time.Duration         `json:"initialReconnectBackoff,omitempty" yaml:"initialReconnectBackoff,omitempty"`
	MaxReconnectBackoff      time.Duration         `json:"maxReconnectBackoff,omitempty" yaml:"maxReconnectBackoff,omitempty"`
	KeepaliveInterval        time.Duration         `json:"keepaliveInterval,omitempty" yaml:"keepaliveInterval,omitempty"`
	KeepaliveTimeout         time.Duration         `json:"keepaliveTimeout,omitempty" yaml:"keepaliveTimeout,omitempty"`
	KeepaliveDrainGrace      time.Duration         `json:"keepaliveDrainGrace,omitempty" yaml:"keepaliveDrainGrace,omitempty"`
	MaxConnectionsPerDomain  int                   `json:"maxConnectionsPerDomain,omitempty" yaml:"maxConnectionsPerDomain,omitempty"`
	MaxConnectionAge         time.Duration         `json:"maxConnectionAge,omitempty" yaml:"maxConnectionAge,omitempty"`
	MaxChannelsPerDomain     int                   `json:"maxChannelsPerDomain,omitempty" yaml:"maxChannelsPerDomain,omitempty"`
	MaxChannelsPerConnection int                   `json:"maxChannelsPerConnection,omitempty" yaml:"maxChannelsPerConnection,omitempty"`
	OnReconnect              func(domainID string) `json:"-" yaml:"-"`
	HostKeyCallback          ssh.HostKeyCallback   `json:"-" yaml:"-"`
}

// DomainInfo identifies a remote SSH endpoint and its credentials.
type DomainInfo struct {
	ID          string
	SSHAddress  string
	Certificate string
	PrivateKey  string
}

func (c *Config) setDefaults() {
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = defaultCleanupInterval
	}
	if c.SSHUser == "" {
		c.SSHUser = defaultSSHUser
	}
	if c.SSHTimeout <= 0 {
		c.SSHTimeout = defaultSSHTimeout
	}
	if c.InitialReconnectBackoff <= 0 {
		c.InitialReconnectBackoff = initialReconnectBackoff
	}
	if c.MaxReconnectBackoff <= 0 {
		c.MaxReconnectBackoff = maxReconnectBackoff
	}
	if c.KeepaliveInterval <= 0 {
		c.KeepaliveInterval = defaultKeepaliveInterval
	}
	if c.KeepaliveTimeout <= 0 {
		c.KeepaliveTimeout = defaultKeepaliveTimeout
	}
	if c.KeepaliveDrainGrace <= 0 {
		c.KeepaliveDrainGrace = defaultKeepaliveDrainGrace
	}
	if c.MaxConnectionsPerDomain <= 0 {
		c.MaxConnectionsPerDomain = defaultMaxConnections
	}
	if c.MaxConnectionAge <= 0 {
		c.MaxConnectionAge = defaultMaxConnectionAge
	}
	if c.MaxChannelsPerDomain <= 0 {
		c.MaxChannelsPerDomain = defaultMaxChannels
	}
	if c.MaxChannelsPerConnection <= 0 {
		c.MaxChannelsPerConnection = defaultMaxChannelsPerConnection
	}
	if c.HostKeyCallback == nil {
		c.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}
}
