package authfeature

import (
	"encoding/base64"
	"errors"
	"strings"
)

// VerifyLoginPassword preserves the existing producer's Argon2id format and
// parameters. Corrupt empty digests must never compare equal to an empty hash.
func VerifyLoginPassword(saved, password string) (bool, error) {
	parts := strings.Split(saved, "$")
	fail := errors.New("invalid stored Login credential")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=1,p=4" {
		return false, fail
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, fail
	}
	digest, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(digest) != 32 {
		return false, fail
	}
	return DefaultArgon2Config().VerifyPassword(saved, password)
}
