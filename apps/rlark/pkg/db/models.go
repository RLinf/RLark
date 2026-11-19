package db

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// RevokedCertificateModel represents a stored model.
type RevokedCertificateModel struct {
	bun.BaseModel `bun:"table:revoked_certificates,alias:rc"`

	// The type of the revoked certificate. e.g., "x509", "ssh", etc.
	CertType string `bun:"cert_type,notnull,pk"`
	// The serial number of the revoked certificate.
	SerialNumber string `bun:"serial_number,notnull,pk"`
	// The subject key ID of the revoked certificate.
	SubjectKeyID string `bun:"subject_key_id,notnull,pk"`
	// The reason for revocation. e.g., "key compromise", "cessation of operation", etc.
	RevocationReason string `bun:"revocation_reason,notnull"`
	// The time when the certificate was revoked.
	RevokedAt time.Time `bun:"revoked_at,notnull"`
}

// RevokedCertificateStore provides access to stored entries.
type RevokedCertificateStore struct {
	db *bun.DB
}

// NewRevokedCertificateStore creates a new RevokedCertificateStore.
func NewRevokedCertificateStore(db *bun.DB) *RevokedCertificateStore {
	return &RevokedCertificateStore{db: db}
}

// AddRevokedCertificate adds the revokedCertificate.
func (s *RevokedCertificateStore) AddRevokedCertificate(ctx context.Context, cert *RevokedCertificateModel) error {
	_, err := s.db.NewInsert().
		Model(cert).
		On("CONFLICT (cert_type, serial_number, subject_key_id) DO NOTHING").
		Exec(ctx)
	return err
}

// IsCertificateRevoked reports whether certificateRevoked.
func (s *RevokedCertificateStore) IsCertificateRevoked(ctx context.Context, certType, serialNumber, subjectKeyID string) (bool, error) {
	count, err := s.db.NewSelect().
		Model((*RevokedCertificateModel)(nil)).
		Where("cert_type = ? AND serial_number = ? AND subject_key_id = ?", certType, serialNumber, subjectKeyID).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SSHUserKeyModel represents a stored model.
type SSHUserKeyModel struct {
	bun.BaseModel `bun:"table:ssh_user_keys,alias:suk"`

	// The unique identifier for the SSH key.
	ID int64 `bun:"id,pk,autoincrement"`
	// The username associated with the SSH key.
	User string `bun:"user,notnull"`
	// The public key in OpenSSH format.
	PublicKey string `bun:"public_key,notnull"`
	// The time when the key was added.
	AddedAt time.Time `bun:"added_at,notnull"`
	// The time when the key was last used.
	LastUsedAt time.Time `bun:"last_used_at"`
	// Notes or description for the SSH key.
	Notes string `bun:"notes"`
}

// SSHUserKeyStore provides access to stored entries.
type SSHUserKeyStore struct {
	db *bun.DB
}

// NewSSHUserKeyStore creates a new SSHUserKeyStore.
func NewSSHUserKeyStore(db *bun.DB) *SSHUserKeyStore {
	return &SSHUserKeyStore{db: db}
}

// AddSSHUserKey adds the sSHUserKey.
func (s *SSHUserKeyStore) AddSSHUserKey(ctx context.Context, key *SSHUserKeyModel) error {
	_, err := s.db.NewInsert().
		Model(key).
		Exec(ctx)
	return err
}

// GetSSHUserKeysByUser returns the sSHUserKeysByUser.
func (s *SSHUserKeyStore) GetSSHUserKeysByUser(ctx context.Context, user string) ([]*SSHUserKeyModel, error) {
	var keys []*SSHUserKeyModel
	err := s.db.NewSelect().
		Model(&keys).
		Where("user = ?", user).
		Order("added_at DESC").
		Scan(ctx)
	return keys, err
}

// UpdateLastUsedAt updates the lastUsedAt.
func (s *SSHUserKeyStore) UpdateLastUsedAt(ctx context.Context, id int64, lastUsedAt time.Time) error {
	_, err := s.db.NewUpdate().
		Model((*SSHUserKeyModel)(nil)).
		Set("last_used_at = ?", lastUsedAt).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// DeleteSSHUserKey deletes the sSHUserKey.
func (s *SSHUserKeyStore) DeleteSSHUserKey(ctx context.Context, id int64) error {
	_, err := s.db.NewDelete().
		Model((*SSHUserKeyModel)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// ListAllSSHUserKeys lists the allSSHUserKeys.
func (s *SSHUserKeyStore) ListAllSSHUserKeys(ctx context.Context, options ListOptions) ([]*SSHUserKeyModel, error) {
	var keys []*SSHUserKeyModel
	query := s.db.NewSelect().
		Model(&keys).
		Order("added_at DESC")

	if options.Limit > 0 {
		query = query.Limit(options.Limit)
	}
	if options.Offset > 0 {
		query = query.Offset(options.Offset)
	}

	err := query.Scan(ctx)
	return keys, err
}
