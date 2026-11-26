package iam

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type PostgresStore struct{ db *bun.DB }

func NewPostgresStore(db *bun.DB) *PostgresStore { return &PostgresStore{db: db} }

func (s *PostgresStore) MigrateAndBootstrap(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin IAM migration transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(731942816)`); err != nil {
		return fmt.Errorf("lock IAM migrations: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS astribot_iam_schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create IAM migration table: %w", err)
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read IAM migrations: %w", err)
	}
	for _, entry := range entries {
		var version int64
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			return fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		var applied bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM astribot_iam_schema_migrations WHERE version=?)`, version).Scan(&applied); err != nil {
			return fmt.Errorf("check IAM migration %d: %w", version, err)
		}
		if applied {
			continue
		}
		sqlText, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read IAM migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(sqlText)); err != nil {
			return fmt.Errorf("apply IAM migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO astribot_iam_schema_migrations(version) VALUES (?)`, version); err != nil {
			return fmt.Errorf("record IAM migration %d: %w", version, err)
		}
	}
	for _, account := range []struct {
		id, username, display string
		role                  Role
		password              string
	}{{"u_admin", "admin", "Administrator", RoleAdmin, "astribot"}, {"u_user", "user", "Operator", RoleOperator, "123"}} {
		hash, err := bcrypt.GenerateFromPassword([]byte(account.password), 12)
		if err != nil {
			return fmt.Errorf("hash bootstrap password: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO astribot_iam_users(id,username,display_name,role,password_hash) VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING`, account.id, account.username, account.display, account.role, string(hash))
		if err != nil {
			return fmt.Errorf("bootstrap IAM user %s: %w", account.username, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit IAM migrations: %w", err)
	}
	return nil
}

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Disabled, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

const userColumns = `id,username,display_name,role,disabled,password_hash,created_at,updated_at,last_login_at`

func (s *PostgresStore) Authenticate(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM astribot_iam_users WHERE username=?`, username))
}
func (s *PostgresStore) GetUser(ctx context.Context, id string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM astribot_iam_users WHERE id=?`, id))
}
func (s *PostgresStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM astribot_iam_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
func (s *PostgresStore) CreateUser(ctx context.Context, u User) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO astribot_iam_users(id,username,display_name,role,disabled,password_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,now(),now())`, u.ID, u.Username, u.DisplayName, u.Role, u.Disabled, u.PasswordHash)
	if isUnique(err) {
		return ErrConflict
	}
	return err
}
func (s *PostgresStore) UpdateUser(ctx context.Context, u User) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE astribot_iam_users SET display_name=?,role=?,disabled=?,updated_at=now() WHERE id=?`, u.DisplayName, u.Role, u.Disabled, u.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if u.Disabled {
		if _, err = tx.ExecContext(ctx, `DELETE FROM astribot_iam_sessions WHERE user_id=?`, u.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *PostgresStore) DeleteUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM astribot_iam_users WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *PostgresStore) SetPassword(ctx context.Context, id, hash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE astribot_iam_users SET password_hash=?,updated_at=now() WHERE id=?`, hash, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM astribot_iam_sessions WHERE user_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *PostgresStore) TouchLogin(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE astribot_iam_users SET last_login_at=?,updated_at=? WHERE id=?`, at, at, id)
	return err
}
func (s *PostgresStore) CreateSession(ctx context.Context, v Session) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO astribot_iam_sessions(id,user_id,created_at,expires_at,last_seen_at,user_agent,ip) VALUES(?,?,?,?,?,?,?)`, v.ID, v.UserID, v.CreatedAt, v.ExpiresAt, v.LastSeenAt, v.UserAgent, v.IP)
	return err
}
func (s *PostgresStore) GetSession(ctx context.Context, id string, now time.Time) (Session, User, error) {
	var session Session
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT s.id,s.user_id,s.created_at,s.expires_at,u.`+strings.ReplaceAll(userColumns, `,`, `,u.`)+` FROM astribot_iam_sessions s JOIN astribot_iam_users u ON u.id=s.user_id WHERE s.id=? AND s.expires_at>?`, id, now).Scan(&session.ID, &session.UserID, &session.CreatedAt, &session.ExpiresAt, &u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Disabled, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, ErrNotFound
	}
	if err == nil {
		if _, updateErr := s.db.ExecContext(ctx, `UPDATE astribot_iam_sessions SET last_seen_at=? WHERE id=?`, now, id); updateErr != nil {
			return Session{}, User{}, updateErr
		}
	}
	return session, u, err
}
func (s *PostgresStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM astribot_iam_sessions WHERE id=?`, id)
	return err
}
func (s *PostgresStore) DeleteUserSessions(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM astribot_iam_sessions WHERE user_id=?`, id)
	return err
}
func (s *PostgresStore) ListSessions(ctx context.Context, userID, currentID string) ([]SessionView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,created_at,last_seen_at,expires_at,user_agent,ip,revoked_at FROM astribot_iam_sessions WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionView{}
	for rows.Next() {
		var v SessionView
		var revoked sql.NullTime
		if err := rows.Scan(&v.ID, &v.CreatedAt, &v.LastSeenAt, &v.ExpiresAt, &v.UserAgent, &v.IP, &revoked); err != nil {
			return nil, err
		}
		v.Current = v.ID == currentID
		if revoked.Valid {
			v.RevokedAt = &revoked.Time
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *PostgresStore) DeleteOtherSessions(ctx context.Context, userID, currentID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM astribot_iam_sessions WHERE user_id=? AND id<>?`, userID, currentID)
	return err
}
func (s *PostgresStore) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM astribot_iam_sessions WHERE expires_at<=?`, now)
	return err
}
func (s *PostgresStore) InsertAudit(ctx context.Context, e AuditEvent) error {
	detail, err := json.Marshal(e.Detail)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO astribot_iam_audit(ts,actor,actor_id,action,target,result,ip,detail) VALUES(COALESCE(?,now()),?,NULLIF(?,''),?,?,?,?,?::jsonb)`, e.Time, e.Actor, e.ActorID, e.Action, e.Target, e.Result, e.IP, string(detail))
	return err
}
func (s *PostgresStore) ListAudit(ctx context.Context, actor, action, result string, limit int) ([]AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,ts,actor,COALESCE(actor_id,''),action,target,result,ip,detail FROM astribot_iam_audit WHERE (?='' OR actor=?) AND (?='' OR action=?) AND (?='' OR result=?) ORDER BY ts DESC,id DESC LIMIT ?`, actor, actor, action, action, result, result, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var detail []byte
		if err := rows.Scan(&e.ID, &e.Time, &e.Actor, &e.ActorID, &e.Action, &e.Target, &e.Result, &e.IP, &detail); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(detail, &e.Detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}

func newID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
