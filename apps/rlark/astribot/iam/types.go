package iam

import (
	"context"
	"time"
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"displayName"`
	Role         Role       `json:"role"`
	Disabled     bool       `json:"disabled"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastLoginAt  *time.Time `json:"lastLoginAt"`
	PasswordHash string     `json:"-"`
}

type Session struct {
	ID         string
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	UserAgent  string
	IP         string
}

type AuditEvent struct {
	ID      int64          `json:"id"`
	Time    time.Time      `json:"time"`
	Actor   string         `json:"actor"`
	ActorID string         `json:"actorId,omitempty"`
	Action  string         `json:"action"`
	Target  string         `json:"target"`
	Result  string         `json:"result"`
	IP      string         `json:"ip"`
	Detail  map[string]any `json:"detail,omitempty"`
}

type Store interface {
	Authenticate(context.Context, string) (User, error)
	GetUser(context.Context, string) (User, error)
	ListUsers(context.Context) ([]User, error)
	CreateUser(context.Context, User) error
	UpdateUser(context.Context, User) error
	DeleteUser(context.Context, string) error
	SetPassword(context.Context, string, string) error
	TouchLogin(context.Context, string, time.Time) error
	CreateSession(context.Context, Session) error
	GetSession(context.Context, string, time.Time) (Session, User, error)
	DeleteSession(context.Context, string) error
	DeleteUserSessions(context.Context, string) error
	ListSessions(context.Context, string, string) ([]SessionView, error)
	DeleteOtherSessions(context.Context, string, string) error
	DeleteExpiredSessions(context.Context, time.Time) error
	InsertAudit(context.Context, AuditEvent) error
	ListAudit(context.Context, string, string, string, int) ([]AuditEvent, error)
}

type SessionView struct {
	ID         string     `json:"id"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	UserAgent  string     `json:"userAgent"`
	IP         string     `json:"ip"`
	Current    bool       `json:"current"`
	RevokedAt  *time.Time `json:"revokedAt"`
}
