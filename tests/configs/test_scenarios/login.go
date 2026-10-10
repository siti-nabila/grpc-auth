package test_scenarios

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/configs"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Login() []testutils.Scenario {
	return []testutils.Scenario{{Name: "Login startup validates key pair before any runtime resource", Run: loginSecurity}, {Name: "Login configuration parsed from existing YAML loader", Run: loginYAML}}
}

func loginSecurity(t *testing.T) {
	priv, pub := fixtures.AccessPEM(t)
	base := configs.AppConfig{ApplicationName: fixtures.AccessIssuer, AccessIssuer: config.AccessIssuerConfig{PrivateKeyFile: "private", PublicKeyFile: "public", KeyVersion: "v1"}, AuthLimiter: config.AuthLimiterConfig{AccountHMACSecret: "test-only-account-secret"}}
	now := func() time.Time { return fixtures.ServiceTime }
	reads := 0
	read := func(path string) ([]byte, error) {
		reads++
		switch path {
		case "private":
			return priv, nil
		case "public":
			return pub, nil
		}
		return nil, errors.New("private-file-detail")
	}
	s, err := configs.LoadLoginSecurity(&base, read, now)
	if err != nil || reads != 2 || s.KeyVersion != "v1" {
		t.Fatal("valid config failed", err)
	}
	secret := s.AccountSecret()
	secret[0] ^= 255
	if !bytes.Equal(s.AccountSecret(), []byte(base.AuthLimiter.AccountHMACSecret)) {
		t.Fatal("secret mutable")
	}
	issued, err := s.Issuer.Issue(jwt.AccessIssue{UserID: 42, SessionID: fixtures.SessionID, TokenID: fixtures.AccessTokenID, AuthVersion: 3, IssuedAt: now(), SessionExpiresAt: now().Add(7 * 24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verifier.Verify(issued.Token); err != nil || reads != 2 {
		t.Fatal("pair not reusable", err)
	}
	changes := []func(*configs.AppConfig){func(c *configs.AppConfig) { c.ApplicationName = "" }, func(c *configs.AppConfig) { c.AccessIssuer.PrivateKeyFile = "" }, func(c *configs.AppConfig) { c.AccessIssuer.PublicKeyFile = "" }, func(c *configs.AppConfig) { c.AccessIssuer.KeyVersion = "" }, func(c *configs.AppConfig) { c.AuthLimiter.AccountHMACSecret = "" }, func(c *configs.AppConfig) { c.JWT.SecretKey = c.AuthLimiter.AccountHMACSecret }, func(c *configs.AppConfig) { c.AccessIssuer.PrivateKeyFile = "missing" }, func(c *configs.AppConfig) { c.AccessIssuer.PublicKeyFile = "private" }}
	for n, change := range changes {
		c := base
		change(&c)
		s, err := configs.LoadLoginSecurity(&c, read, now)
		if err == nil || s != nil || strings.Contains(err.Error(), "private-file-detail") {
			t.Fatal("unsafe config accepted", n)
		}
	}
	if _, err := configs.LoadLoginSecurity(nil, read, now); err == nil {
		t.Fatal("nil config")
	}
	if _, err := configs.LoadLoginSecurity(&base, nil, now); err == nil {
		t.Fatal("nil reader")
	}
	if _, err := configs.LoadLoginSecurity(&base, read, nil); err == nil {
		t.Fatal("nil clock")
	}
}

func loginYAML(t *testing.T) {
	data := `ApplicationName: auth-test
TLS: {CertFile: certificate, KeyFile: private}
ServiceAuth:
  Callers:
    - ServiceID: rest-orc
      Current: {SHA256: "` + fixtures.ServiceDigest(fixtures.ServiceToken(1)) + `", NotAfter: "2035-01-01T00:00:00Z"}
      Methods: [/user.v1.UserService/Login]
      Namespaces: [iam]
AccessIssuer: {PrivateKeyFile: issuer-private, PublicKeyFile: issuer-public, KeyVersion: v1}
AuthLimiter: {AccountHMACSecret: test-only-account-secret}
`
	c, err := configs.ReadConfig(strings.NewReader(data), fixtures.ServiceTime)
	if err != nil || c.AccessIssuer.PrivateKeyFile != "issuer-private" || c.AuthLimiter.AccountHMACSecret != "test-only-account-secret" {
		t.Fatal("config parse failed", err)
	}
	bad := strings.Replace(data, "KeyVersion: v1", "KeyVersion: [invalid]", 1)
	if _, err := configs.ReadConfig(strings.NewReader(bad), fixtures.ServiceTime); err == nil {
		t.Fatal("bad types accepted")
	}
}
