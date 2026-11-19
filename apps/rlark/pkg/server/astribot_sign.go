package server

import (
	"context"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

type certificateRevocationStore interface {
	AddRevokedCertificate(context.Context, *db.RevokedCertificateModel) error
	IsCertificateRevoked(context.Context, string, string, string) (bool, error)
}
