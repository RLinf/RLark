package server

import (
	"context"
	"encoding/hex"
	"math/big"
	"strconv"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// certificateRevocationStore is what the server needs of the revocation store (db.RevokedCertificateStore), so
// that tests can stand in for the database.
type certificateRevocationStore interface {
	AddRevokedCertificate(context.Context, *db.RevokedCertificateModel) error
	IsCertificateRevoked(context.Context, string, string, string) (bool, error)
}

// invalidRevocation says what is wrong with a revocation's certificate identity, or nothing. A revocation is stored
// as the identity the certificate check looks up (decimal serial, lowercase hex subject key ID for x509), so one
// written any other way would never match the certificate it names.
func invalidRevocation(req RevokeCertRequest) string {
	if (req.CertType != "x509" && req.CertType != "ssh") || req.SerialNumber == "" || req.SubjectKeyID == "" || len(req.SubjectKeyID) > 512 || len(req.Reason) > 1024 || strings.TrimSpace(req.SubjectKeyID) != req.SubjectKeyID {
		return "invalid certificate identity"
	}
	if req.CertType == "ssh" {
		serial, err := strconv.ParseUint(req.SerialNumber, 10, 64)
		if err != nil || strconv.FormatUint(serial, 10) != req.SerialNumber {
			return "invalid certificate serial"
		}
		return ""
	}
	serial, ok := new(big.Int).SetString(req.SerialNumber, 10)
	if !ok || serial.Sign() <= 0 || serial.BitLen() > 160 || serial.String() != req.SerialNumber {
		return "invalid certificate serial"
	}
	decoded, err := hex.DecodeString(req.SubjectKeyID)
	if err != nil || len(decoded) == 0 || hex.EncodeToString(decoded) != req.SubjectKeyID {
		return "invalid certificate subject key ID"
	}
	return ""
}
