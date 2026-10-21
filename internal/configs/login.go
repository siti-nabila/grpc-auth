package configs

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

type LoginSecurity struct {
	Issuer        *jwt.AccessIssuer
	Verifier      *jwt.AccessVerifier
	KeyVersion    string
	accountSecret []byte
}

func (s *LoginSecurity) AccountSecret() []byte { return append([]byte(nil), s.accountSecret...) }

// LoadLoginSecurity validates issuer and limiter material before startup opens
// any resource. The default foundation pipeline uses this issuer/verifier pair.
func LoadLoginSecurity(c *AppConfig, readFile func(string) ([]byte, error), now func() time.Time) (*LoginSecurity, error) {
	fail := errors.New("invalid Login issuer or limiter configuration")
	if c == nil || readFile == nil || now == nil || strings.TrimSpace(c.ApplicationName) == "" || strings.TrimSpace(c.AccessIssuer.PrivateKeyFile) == "" || strings.TrimSpace(c.AccessIssuer.PublicKeyFile) == "" || strings.TrimSpace(c.AccessIssuer.KeyVersion) == "" || c.AuthLimiter.AccountHMACSecret == "" {
		return nil, fail
	}
	if c.JWT.SecretKey != "" && c.AuthLimiter.AccountHMACSecret == c.JWT.SecretKey {
		return nil, fail
	}
	privPEM, err := readFile(c.AccessIssuer.PrivateKeyFile)
	if err != nil {
		return nil, fail
	}
	pubPEM, err := readFile(c.AccessIssuer.PublicKeyFile)
	if err != nil {
		return nil, fail
	}
	priv, pub, err := jwt.ParseEd25519Keys(privPEM, pubPEM)
	if err != nil {
		return nil, fail
	}
	probe := []byte("grpc-auth issuer configuration check")
	if !ed25519.Verify(pub, probe, ed25519.Sign(priv, probe)) {
		return nil, fail
	}
	issuer, err := jwt.NewAccessIssuer(c.ApplicationName, priv)
	if err != nil {
		return nil, fail
	}
	verifier, err := jwt.NewAccessVerifier(c.ApplicationName, pub, now)
	if err != nil {
		return nil, fail
	}
	return &LoginSecurity{Issuer: issuer, Verifier: verifier, KeyVersion: c.AccessIssuer.KeyVersion, accountSecret: []byte(c.AuthLimiter.AccountHMACSecret)}, nil
}
