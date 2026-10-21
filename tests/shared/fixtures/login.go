package fixtures

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Same format/parameters as production, with a fixed test-only salt.
func LoginPasswordHash() string { return LoginPasswordHashFor("secret12") }

func LoginPasswordHashFor(password string) string {
	salt := bytes.Repeat([]byte{2}, 16)
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}
