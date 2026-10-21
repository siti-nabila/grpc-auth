package test_scenarios

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	jwtPackage "github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "EdDSA access verifier requires exact claims and rejects legacy algorithms", Run: accessValidation},
		{Name: "access skew and lifetime boundaries use controlled clock and ignore audience", Run: accessTimes},
	}
}

func accessValidation(t *testing.T) {
	key := fixtures.AccessKey().Public().(ed25519.PublicKey)
	v, err := jwtPackage.NewAccessVerifier(fixtures.AccessIssuer, key, func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	key[0] ^= 255 // Constructor owns an immutable key copy.
	got, err := v.Verify(fixtures.SignAccess(t, fixtures.AccessPayload()))
	if err != nil || got.UserID() != 42 || got.SessionID() != fixtures.SessionID || got.TokenID() != fixtures.AccessTokenID || got.AuthVersion() != 3 || !got.ExpiresAt().Equal(fixtures.ServiceTime.Add(14*time.Minute)) {
		t.Fatal("valid claims not preserved", err)
	}
	for _, field := range []string{"sub", "sid", "jti", "iss", "iat", "exp", "auth_version"} {
		t.Run("missing "+field, func(t *testing.T) {
			p := fixtures.AccessPayload()
			delete(p, field)
			if _, err := v.Verify(fixtures.SignAccess(t, p)); !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("missing claim accepted")
			}
		})
	}
	cases := []struct {
		name, field string
		value       any
	}{
		{"wrong issuer", "iss", "another"}, {"empty issuer", "iss", ""}, {"zero sub", "sub", "0"}, {"nondecimal sub", "sub", "42x"}, {"noncanonical sub", "sub", "042"}, {"signed sub", "sub", "+42"}, {"ID overflow", "sub", "2147483648"}, {"numeric sub", "sub", 42},
		{"invalid sid", "sid", "not-a-uuid"}, {"invalid jti", "jti", "not-a-uuid"}, {"uppercase UUID", "sid", "00000000-0000-4000-ABCD-000000000001"},
		{"zero auth version", "auth_version", 0}, {"negative auth version", "auth_version", -1}, {"fraction auth version", "auth_version", 1.5}, {"string auth version", "auth_version", "3"},
		{"legacy mismatch", "user_id", 43}, {"future iat", "iat", fixtures.ServiceTime.Add(31 * time.Second).Unix()}, {"malformed iat", "iat", "tomorrow"}, {"malformed exp", "exp", "tomorrow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := fixtures.AccessPayload()
			p[tc.field] = tc.value
			if _, err := v.Verify(fixtures.SignAccess(t, p)); !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("invalid claim accepted")
			}
		})
	}
	legacy, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, fixtures.AccessPayload()).SignedString([]byte("test-only-key"))
	wrong, _ := jwt.NewWithClaims(jwt.SigningMethodEdDSA, fixtures.AccessPayload()).SignedString(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{10}, 32)))
	for _, token := range []string{legacy, wrong, "", "malformed"} {
		if _, err := v.Verify(token); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatal("wrong signature/algorithm accepted")
		}
	}
	if _, err := jwtPackage.NewAccessVerifier("", nil, nil); err == nil {
		t.Fatal("missing verifier configuration accepted")
	}
}

func accessTimes(t *testing.T) {
	now := fixtures.ServiceTime
	v, err := jwtPackage.NewAccessVerifier(fixtures.AccessIssuer, fixtures.AccessKey().Public().(ed25519.PublicKey), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	for _, aud := range []any{nil, "another", []string{"another", "unrelated"}} {
		p := fixtures.AccessPayload()
		if aud != nil {
			p["aud"] = aud
		}
		p["user_id"] = 42
		if _, err := v.Verify(fixtures.SignAccess(t, p)); err != nil {
			t.Fatal("audience incorrectly required or legacy ID match rejected", err)
		}
	}
	for _, seconds := range []int64{0, 1, 900, 901} {
		p := fixtures.AccessPayload()
		p["iat"] = now.Unix()
		p["exp"] = now.Unix() + seconds
		_, err := v.Verify(fixtures.SignAccess(t, p))
		valid := seconds > 0 && seconds <= 900
		if valid && err != nil || !valid && !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatal("access duration boundary", seconds, err)
		}
	}
	p := fixtures.AccessPayload()
	encoded := fixtures.SignAccess(t, p)
	exp := time.Unix(p["exp"].(int64), 0)
	for _, point := range []time.Time{exp, exp.Add(30*time.Second - time.Nanosecond), exp.Add(30 * time.Second)} {
		now = point
		_, err := v.Verify(encoded)
		valid := point.Before(exp.Add(30 * time.Second))
		if valid && err != nil || !valid && !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatal("JWT skew boundary", point, err)
		}
	}
}
