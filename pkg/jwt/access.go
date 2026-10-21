package jwt

import (
	"crypto/ed25519"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
)

const (
	MaxAccessLifetime  = 15 * time.Minute
	MaxSessionLifetime = 7 * 24 * time.Hour
	AccessClockSkew    = 30 * time.Second
)

var accessUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type accessClaims struct {
	SessionID    string  `json:"sid"`
	AuthVersion  *int64  `json:"auth_version"`
	LegacyUserID *uint64 `json:"user_id,omitempty"`
	jwt.RegisteredClaims
}

// VerifiedClaims has no mutable exported state or role/permission grants.
type VerifiedClaims struct {
	userID              uint64
	sessionID, tokenID  string
	authVersion         int64
	issuedAt, expiresAt time.Time
}

func (c VerifiedClaims) UserID() uint64       { return c.userID }
func (c VerifiedClaims) SessionID() string    { return c.sessionID }
func (c VerifiedClaims) TokenID() string      { return c.tokenID }
func (c VerifiedClaims) AuthVersion() int64   { return c.authVersion }
func (c VerifiedClaims) IssuedAt() time.Time  { return c.issuedAt }
func (c VerifiedClaims) ExpiresAt() time.Time { return c.expiresAt }

type AccessVerifier struct {
	key    ed25519.PublicKey
	issuer string
	now    func() time.Time
}

func NewAccessVerifier(issuer string, key ed25519.PublicKey, now func() time.Time) (*AccessVerifier, error) {
	if issuer == "" || len(key) != ed25519.PublicKeySize || now == nil {
		return nil, errors.New("access verifier requires issuer Ed25519 public key and clock")
	}
	return &AccessVerifier{issuer: issuer, key: append(ed25519.PublicKey(nil), key...), now: now}, nil
}

func (v *AccessVerifier) Verify(encoded string) (VerifiedClaims, error) {
	claims := new(accessClaims)
	token, err := jwt.ParseWithClaims(encoded, claims, func(*jwt.Token) (any, error) { return v.key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(AccessClockSkew), jwt.WithTimeFunc(v.now))
	if err != nil || !token.Valid || claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.AuthVersion == nil || *claims.AuthVersion < 1 {
		return VerifiedClaims{}, auth.ErrInvalidSession
	}
	id, err := strconv.ParseUint(claims.Subject, 10, 32)
	if err != nil || id == 0 || id > 2147483647 || strconv.FormatUint(id, 10) != claims.Subject || !accessUUID.MatchString(claims.SessionID) || !accessUUID.MatchString(claims.ID) {
		return VerifiedClaims{}, auth.ErrInvalidSession
	}
	if claims.LegacyUserID != nil && *claims.LegacyUserID != id {
		return VerifiedClaims{}, auth.ErrInvalidSession
	}
	iat, exp := claims.IssuedAt.Time, claims.ExpiresAt.Time
	if iat.Unix() <= 0 || !exp.After(iat) || exp.Sub(iat) > MaxAccessLifetime {
		return VerifiedClaims{}, auth.ErrInvalidSession
	}
	return VerifiedClaims{userID: id, sessionID: claims.SessionID, tokenID: claims.ID, authVersion: *claims.AuthVersion, issuedAt: iat, expiresAt: exp}, nil
}
