package iam

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"golang.org/x/crypto/bcrypt"
)

func TestPostgresMigrationBootstrapIdempotent(t *testing.T) {
	adminDSN := os.Getenv("ASTRIBOT_IAM_TEST_ADMIN_DSN")
	if adminDSN == "" {
		t.Skip("ASTRIBOT_IAM_TEST_ADMIN_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, adminSQL := openTestDB(t, adminDSN)
	databaseName := fmt.Sprintf("astribot_iam_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+databaseName); err != nil {
		t.Fatalf("create isolated test database: %v", err)
	}
	defer admin.Close()
	defer func() { _, _ = adminSQL.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+databaseName) }()
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse PostgreSQL test DSN: %v", err)
	}
	parsed.Path = "/" + databaseName
	testDSN := parsed.String()
	testDB, _ := openTestDB(t, testDSN)
	defer testDB.Close()
	store := NewPostgresStore(testDB)
	if err := store.MigrateAndBootstrap(ctx); err != nil {
		t.Fatalf("initial migration/bootstrap: %v", err)
	}
	if err := store.MigrateAndBootstrap(ctx); err != nil {
		t.Fatalf("repeat migration/bootstrap: %v", err)
	}
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("bootstrap user count=%d, want 2", len(users))
	}
	for _, check := range []struct{ name, password string }{{"admin", "astribot"}, {"user", "123"}} {
		u, err := store.Authenticate(ctx, check.name)
		if err != nil {
			t.Fatal(err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(check.password)); err != nil {
			t.Fatalf("bootstrap password for %s is not bcrypt-verifiable: %v", check.name, err)
		}
	}
	if err := store.CreateUser(ctx, User{ID: "duplicate", Username: "admin", DisplayName: "Duplicate", Role: RoleOperator, PasswordHash: "unused"}); err != ErrConflict {
		t.Fatalf("duplicate username error=%v, want ErrConflict", err)
	}
	created := User{ID: "u_integration", Username: "integration", DisplayName: "Integration", Role: RoleViewer, PasswordHash: "bcrypt-hash"}
	if err := store.CreateUser(ctx, created); err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now().UTC()
	if err := store.TouchLogin(ctx, created.ID, now); err != nil {
		t.Fatalf("touch login: %v", err)
	}
	if got, err := store.GetUser(ctx, created.ID); err != nil || got.LastLoginAt == nil {
		t.Fatalf("read updated last login: %#v, %v", got, err)
	}
	session := Session{ID: strings.Repeat("a", 64), UserID: created.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour), UserAgent: "test-agent", IP: "192.0.2.4"}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, user, err := store.GetSession(ctx, session.ID, now); err != nil || user.ID != created.ID {
		t.Fatalf("get session: user=%q err=%v", user.ID, err)
	}
	sessions, err := store.ListSessions(ctx, created.ID, session.ID)
	if err != nil || len(sessions) != 1 || !sessions[0].Current || sessions[0].UserAgent != "test-agent" {
		t.Fatalf("list sessions: %#v, %v", sessions, err)
	}
	if err := store.DeleteOtherSessions(ctx, created.ID, "other-session"); err != nil {
		t.Fatalf("delete other sessions: %v", err)
	}
	if _, _, err := store.GetSession(ctx, session.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked session lookup error=%v", err)
	}
	if err := store.InsertAudit(ctx, AuditEvent{Time: now, Actor: "admin", ActorID: "u_admin", Action: "user-create", Target: created.Username, Result: "success", IP: "192.0.2.1", Detail: map[string]any{"source": "test"}}); err != nil {
		t.Fatalf("insert audit: %v", err)
	}
	events, err := store.ListAudit(ctx, "admin", "user-create", "success", 10)
	if err != nil || len(events) != 1 || events[0].Detail["source"] != "test" {
		t.Fatalf("list audit: %#v, %v", events, err)
	}
	if err := store.UpdateUser(ctx, User{ID: created.ID, DisplayName: "Changed", Role: RoleOperator, Disabled: true}); err != nil {
		t.Fatalf("update/disable user: %v", err)
	}
	if got, err := store.GetUser(ctx, created.ID); err != nil || !got.Disabled || got.DisplayName != "Changed" {
		t.Fatalf("read updated user: %#v, %v", got, err)
	}
	if err := store.SetPassword(ctx, created.ID, "new-bcrypt-hash"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if err := store.DeleteUser(ctx, created.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
}

func openTestDB(t *testing.T, dsn string) (*bun.DB, *sql.DB) {
	t.Helper()
	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn), pgdriver.WithTimeout(5*time.Second)))
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	return bun.NewDB(sqlDB, pgdialect.New()), sqlDB
}
