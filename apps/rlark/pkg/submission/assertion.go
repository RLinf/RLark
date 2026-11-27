// Package submission defines the internal IAM assertion for workload creation.
package submission

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// Header carries the assertion through the console proxy.
	Header   = "X-RLark-Submitter"
	issuer   = "astribot-iam"
	audience = "rlark-workload-submission"
	lifetime = 30 * time.Second
)

// Claims binds the human identity to an authenticated Gateway account and create route.
type Claims struct {
	Username string `json:"username"`
	Account  string `json:"account"`
	Path     string `json:"path"`
	jwt.RegisteredClaims
}

// Sign asserts the identity of a human who has just authorized this workload submission.
func Sign(key ed25519.PrivateKey, id, username, account, path string, now time.Time) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, Claims{
		Username: username, Account: account, Path: path,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			Subject:   id,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(lifetime)),
		},
	}).SignedString(key)
}

// Verify validates an assertion for a caller whose service account has already been authenticated.
func Verify(key ed25519.PublicKey, raw, account, path string) (Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithIssuer(issuer),
		jwt.WithAudience(audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		return Claims{}, err
	}
	if claims.IssuedAt == nil || claims.ExpiresAt.Sub(claims.IssuedAt.Time) != lifetime ||
		claims.Subject == "" || claims.Username == "" || claims.Account != account || claims.Path != path {
		return Claims{}, errors.New("submission assertion does not match request")
	}
	return claims, nil
}
