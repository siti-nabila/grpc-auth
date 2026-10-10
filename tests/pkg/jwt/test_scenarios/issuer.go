package test_scenarios

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	jwtPackage "github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Issuer() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "issuer interoperates with foundation and clamps access deadline", Run: issuerLifetime},
		{Name: "issuer rejects invalid input and owns key bytes", Run: issuerValidation},
		{Name: "PEM parser rejects malformed or mismatched keys", Run: issuerKeys},
	}
}

func issuerLifetime(t *testing.T) {
	key := fixtures.AccessKey()
	i, err := jwtPackage.NewAccessIssuer(fixtures.AccessIssuer, key)
	if err != nil {
		t.Fatal(err)
	}
	v, err := jwtPackage.NewAccessVerifier(fixtures.AccessIssuer, key.Public().(ed25519.PublicKey), func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	key[0] ^= 255
	for _, duration := range []time.Duration{7 * 24 * time.Hour, 5 * time.Minute} {
		req := jwtPackage.AccessIssue{UserID: 42, SessionID: fixtures.SessionID, TokenID: fixtures.AccessTokenID, AuthVersion: 3, IssuedAt: fixtures.ServiceTime, SessionExpiresAt: fixtures.ServiceTime.Add(duration)}
		issued, err := i.Issue(req)
		if err != nil {
			t.Fatal(err)
		}
		claims, err := v.Verify(issued.Token)
		want := fixtures.ServiceTime.Add(15 * time.Minute)
		if duration < 15*time.Minute {
			want = fixtures.ServiceTime.Add(duration)
		}
		if err != nil || claims.UserID() != 42 || claims.AuthVersion() != 3 || claims.SessionID() != req.SessionID || claims.TokenID() != req.TokenID || !claims.IssuedAt().Equal(req.IssuedAt) || !claims.ExpiresAt().Equal(want) || !issued.ExpiresAt.Equal(want) {
			t.Fatal("issuer/verifier mismatch", err)
		}
	}
}

func issuerValidation(t *testing.T) {
	i, err := jwtPackage.NewAccessIssuer(fixtures.AccessIssuer, fixtures.AccessKey())
	if err != nil {
		t.Fatal(err)
	}
	base := jwtPackage.AccessIssue{UserID: 42, SessionID: fixtures.SessionID, TokenID: fixtures.AccessTokenID, AuthVersion: 3, IssuedAt: fixtures.ServiceTime, SessionExpiresAt: fixtures.ServiceTime.Add(7 * 24 * time.Hour)}
	changes := []func(*jwtPackage.AccessIssue){
		func(r *jwtPackage.AccessIssue) { r.UserID = 0 }, func(r *jwtPackage.AccessIssue) { r.UserID = 2147483648 }, func(r *jwtPackage.AccessIssue) { r.AuthVersion = 0 },
		func(r *jwtPackage.AccessIssue) { r.SessionID = "invalid" }, func(r *jwtPackage.AccessIssue) { r.TokenID = "invalid" }, func(r *jwtPackage.AccessIssue) { r.IssuedAt = time.Time{} },
		func(r *jwtPackage.AccessIssue) { r.SessionExpiresAt = r.IssuedAt }, func(r *jwtPackage.AccessIssue) { r.SessionExpiresAt = r.IssuedAt.Add(7*24*time.Hour + time.Second) },
	}
	for n, change := range changes {
		r := base
		change(&r)
		out, err := i.Issue(r)
		if !errors.Is(err, auth.ErrInvalidSession) || out.Token != "" {
			t.Fatal("invalid issuance accepted", n)
		}
	}
	if _, err := jwtPackage.NewAccessIssuer("", fixtures.AccessKey()); err == nil {
		t.Fatal("missing issuer accepted")
	}
	if _, err := jwtPackage.NewAccessIssuer("issuer", nil); err == nil {
		t.Fatal("missing key accepted")
	}
	bad := fixtures.AccessKey()
	bad[63] ^= 255
	if _, err := jwtPackage.NewAccessIssuer("issuer", bad); err == nil {
		t.Fatal("corrupt key accepted")
	}
}

func issuerKeys(t *testing.T) {
	priv, pub := fixtures.AccessPEM(t)
	key, public, err := jwtPackage.ParseEd25519Keys(priv, pub)
	if err != nil || !bytes.Equal(key, fixtures.AccessKey()) || !bytes.Equal(public, fixtures.AccessKey().Public().(ed25519.PublicKey)) {
		t.Fatal("valid pair rejected", err)
	}
	other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{10}, 32))
	otherDER, err := x509.MarshalPKIXPublicKey(other.Public())
	if err != nil {
		t.Fatal(err)
	}
	otherPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: otherDER})
	for _, pair := range [][2][]byte{{priv, otherPEM}, {nil, pub}, {priv, nil}, {[]byte("private-secret"), pub}, {pub, pub}, {priv, priv}, {append(append([]byte{}, priv...), priv...), pub}} {
		if _, _, err := jwtPackage.ParseEd25519Keys(pair[0], pair[1]); err == nil {
			t.Fatal("invalid key pair accepted")
		}
	}
}
