package jwt

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strconv"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
)

type AccessIssue struct {
	UserID                     uint64
	SessionID, TokenID         string
	AuthVersion                int64
	IssuedAt, SessionExpiresAt time.Time
}

type IssuedAccess struct {
	Token     string
	ExpiresAt time.Time
}

type AccessIssuer struct {
	issuer string
	key    ed25519.PrivateKey
}

func NewAccessIssuer(issuer string, key ed25519.PrivateKey) (*AccessIssuer, error) {
	if issuer == "" || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("access issuer requires issuer and Ed25519 private key")
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(key.Seed()), key) {
		return nil, errors.New("invalid Ed25519 private key")
	}
	return &AccessIssuer{issuer: issuer, key: append(ed25519.PrivateKey(nil), key...)}, nil
}

func (i *AccessIssuer) Issue(req AccessIssue) (IssuedAccess, error) {
	issued := req.IssuedAt.UTC().Truncate(time.Second)
	deadline := req.SessionExpiresAt.UTC().Truncate(time.Second)
	if req.UserID == 0 || req.UserID > 2147483647 || req.AuthVersion < 1 || !accessUUID.MatchString(req.SessionID) || !accessUUID.MatchString(req.TokenID) || issued.Unix() <= 0 || !deadline.After(issued) || deadline.Sub(issued) > MaxSessionLifetime {
		return IssuedAccess{}, auth.ErrInvalidSession
	}
	expires := issued.Add(MaxAccessLifetime)
	if deadline.Before(expires) {
		expires = deadline
	}
	claims := accessClaims{
		SessionID: req.SessionID, AuthVersion: &req.AuthVersion,
		RegisteredClaims: jwtlib.RegisteredClaims{Subject: strconv.FormatUint(req.UserID, 10), ID: req.TokenID, Issuer: i.issuer, IssuedAt: jwtlib.NewNumericDate(issued), ExpiresAt: jwtlib.NewNumericDate(expires)},
	}
	encoded, err := jwtlib.NewWithClaims(jwtlib.SigningMethodEdDSA, claims).SignedString(i.key)
	if err != nil {
		return IssuedAccess{}, err
	}
	return IssuedAccess{Token: encoded, ExpiresAt: expires}, nil
}

// ParseEd25519Keys accepts one PKCS#8 private and one PKIX public PEM block.
// Errors deliberately contain no input material or parser-specific detail.
func ParseEd25519Keys(privatePEM, publicPEM []byte) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	fail := errors.New("invalid or mismatched Ed25519 key pair")
	privBlock, privRest := pem.Decode(privatePEM)
	pubBlock, pubRest := pem.Decode(publicPEM)
	if privBlock == nil || pubBlock == nil || privBlock.Type != "PRIVATE KEY" || pubBlock.Type != "PUBLIC KEY" || len(bytes.TrimSpace(privRest)) != 0 || len(bytes.TrimSpace(pubRest)) != 0 {
		return nil, nil, fail
	}
	privRaw, err := x509.ParsePKCS8PrivateKey(privBlock.Bytes)
	if err != nil {
		return nil, nil, fail
	}
	pubRaw, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return nil, nil, fail
	}
	priv, ok := privRaw.(ed25519.PrivateKey)
	if !ok {
		return nil, nil, fail
	}
	pub, ok := pubRaw.(ed25519.PublicKey)
	if !ok || !bytes.Equal(priv.Public().(ed25519.PublicKey), pub) {
		return nil, nil, fail
	}
	return append(ed25519.PrivateKey(nil), priv...), append(ed25519.PublicKey(nil), pub...), nil
}
