package test_scenarios

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"strings"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Credentials() []testutils.Scenario {
	return []testutils.Scenario{{Name: "initial credentials use independent UUID entropy and only encoded-token digest", Run: initialCredentials}, {Name: "entropy failure never returns partial credentials", Run: credentialFailures}}
}

func initialCredentials(t *testing.T) {
	data := make([]byte, 96)
	for n := range data {
		data[n] = byte(n)
	}
	c, err := authfeature.GenerateInitialCredentials(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, id := range []string{c.SessionID, c.RefreshID, c.FamilyID, c.TokenID} {
		if seen[id] || len(id) != 36 || id[14] != '4' || !strings.ContainsRune("89ab", rune(id[19])) {
			t.Fatal("independent UUIDv4 not produced")
		}
		seen[id] = true
	}
	if c.SessionID != "00010203-0405-4607-8809-0a0b0c0d0e0f" {
		t.Fatal("UUID encoding mismatch")
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.RefreshToken)
	want := sha256.Sum256([]byte(c.RefreshToken))
	if err != nil || !bytes.Equal(raw, data[64:]) || !bytes.Equal(c.RefreshDigest, want[:]) {
		t.Fatal("refresh digest encoding mismatch")
	}
}

func credentialFailures(t *testing.T) {
	for _, n := range []int{0, 15, 16, 31, 32, 47, 48, 63, 64, 95} {
		c, err := authfeature.GenerateInitialCredentials(bytes.NewReader(make([]byte, n)))
		if !errors.Is(err, common.ErrDependencyUnavailable) || c.SessionID != "" || c.RefreshToken != "" || len(c.RefreshDigest) != 0 {
			t.Fatal("partial credential leaked", n)
		}
	}
	if _, err := authfeature.GenerateInitialCredentials(nil); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("nil entropy accepted")
	}
}

func LoginPassword() []testutils.Scenario {
	return []testutils.Scenario{{Name: "production Login rejects malformed stored Argon2 digest without weakening existing format", Run: loginPassword}}
}
func loginPassword(t *testing.T) {
	hash := fixtures.LoginPasswordHash()
	ok, err := authfeature.VerifyLoginPassword(hash, "secret12")
	if err != nil || !ok {
		t.Fatal("valid password rejected", err)
	}
	ok, err = authfeature.VerifyLoginPassword(hash, "wrong-password")
	if err != nil || ok {
		t.Fatal("mismatch accepted", err)
	}
	parts := strings.Split(hash, "$")
	for _, bad := range []string{"", "plain", strings.Join(parts[:5], "$") + "$", strings.Replace(hash, "v=19", "v=18", 1), strings.Replace(hash, "argon2id", "argon2i", 1), strings.Replace(hash, parts[4], "bad", 1)} {
		if ok, err := authfeature.VerifyLoginPassword(bad, "secret12"); err == nil || ok {
			t.Fatal("malformed stored credential accepted")
		}
	}
}
