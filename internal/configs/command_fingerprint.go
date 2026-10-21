package configs

import (
	"crypto/subtle"
	"errors"

	"github.com/siti-nabila/grpc-auth/internal/features/common"
)

// LoadCommandFingerprinter validates secret separation before resources open.
// The feature receives a copied, immutable keyring and never reads configuration.
func LoadCommandFingerprinter(c *AppConfig) (*common.Fingerprinter, error) {
	fail := errors.New("invalid command fingerprint configuration")
	if c == nil {
		return nil, fail
	}
	keys := make(map[string][]byte, len(c.CommandFingerprint.Keys))
	for version, secret := range c.CommandFingerprint.Keys {
		if secret == "" || sameSecret(secret, c.JWT.SecretKey) || sameSecret(secret, c.AuthLimiter.AccountHMACSecret) {
			return nil, fail
		}
		for _, previous := range keys {
			if subtle.ConstantTimeCompare([]byte(secret), previous) == 1 {
				return nil, fail
			}
		}
		keys[version] = []byte(secret)
	}
	f, err := common.NewFingerprinter(c.CommandFingerprint.ActiveVersion, keys)
	if err != nil {
		return nil, fail
	}
	return f, nil
}

func sameSecret(a, b string) bool {
	return b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
