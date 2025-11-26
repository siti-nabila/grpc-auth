package authfeature

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type (
	ArgonConfig struct {
		Memory     uint32
		Time       uint32
		Threads    uint8
		KeyLength  uint32
		SaltLength uint32
	}
)

func DefaultArgon2Config() *ArgonConfig {
	return &ArgonConfig{
		Memory:     64 * 1024, // 64 MB
		Time:       1,
		Threads:    4,
		KeyLength:  32,
		SaltLength: 16,
	}
}

func (c *ArgonConfig) HashPassword(password string) (result string, err error) {
	salt := make([]byte, c.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, c.Time, c.Memory, c.Threads, c.KeyLength)
	base64Salt := base64.RawStdEncoding.EncodeToString(salt)
	base64Hash := base64.RawStdEncoding.EncodeToString(hash)

	result = fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		c.Memory, c.Time, c.Threads,
		base64Salt, base64Hash,
	)

	return result, nil

}

func (c *ArgonConfig) VerifyPassword(passwordSaved, passwordRequest string) (match bool, err error) {
	parts := strings.Split(passwordSaved, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid password format")
	}

	base64Salt := parts[4]
	base64Hash := parts[5]

	salt, err := base64.RawStdEncoding.DecodeString(base64Salt)
	if err != nil {
		return false, err
	}

	hash, err := base64.RawStdEncoding.DecodeString(base64Hash)
	if err != nil {
		return false, err
	}

	newHash := argon2.IDKey([]byte(passwordRequest), salt, c.Time, c.Memory, c.Threads, uint32(len(hash)))

	// compare hash
	if subtle.ConstantTimeCompare(hash, newHash) == 1 {
		return true, nil

	}

	return false, nil

}
