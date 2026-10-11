package authfeature

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
)

type InitialCredentials struct {
	SessionID, RefreshID, FamilyID, TokenID string
	RefreshToken                            string
	RefreshDigest                           []byte
}

// RotatedCredentials does not allocate a new session or family.
type RotatedCredentials struct {
	RefreshID, TokenID, RefreshToken string
	RefreshDigest                    []byte
}

func GenerateRotatedCredentials(entropy io.Reader) (RotatedCredentials, error) {
	id, err := randomUUID(entropy)
	if err != nil {
		return RotatedCredentials{}, err
	}
	tokenID, err := randomUUID(entropy)
	if err != nil {
		return RotatedCredentials{}, err
	}
	var secret [32]byte
	if _, err = io.ReadFull(entropy, secret[:]); err != nil {
		return RotatedCredentials{}, fmt.Errorf("%w: %w", common.ErrDependencyUnavailable, err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret[:])
	digest := sha256.Sum256([]byte(encoded))
	return RotatedCredentials{RefreshID: id, TokenID: tokenID, RefreshToken: encoded, RefreshDigest: append([]byte(nil), digest[:]...)}, nil
}

func GenerateInitialCredentials(entropy io.Reader) (InitialCredentials, error) {
	if entropy == nil {
		return InitialCredentials{}, common.ErrDependencyUnavailable
	}
	var ids [4]string
	for n := range ids {
		id, err := randomUUID(entropy)
		if err != nil {
			return InitialCredentials{}, err
		}
		ids[n] = id
	}
	var secret [32]byte
	if _, err := io.ReadFull(entropy, secret[:]); err != nil {
		return InitialCredentials{}, fmt.Errorf("%w: %w", common.ErrDependencyUnavailable, err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret[:])
	digest := sha256.Sum256([]byte(encoded))
	return InitialCredentials{SessionID: ids[0], RefreshID: ids[1], FamilyID: ids[2], TokenID: ids[3], RefreshToken: encoded, RefreshDigest: append([]byte(nil), digest[:]...)}, nil
}

func randomUUID(entropy io.Reader) (string, error) {
	if entropy == nil {
		return "", common.ErrDependencyUnavailable
	}
	var b [16]byte
	if _, err := io.ReadFull(entropy, b[:]); err != nil {
		return "", fmt.Errorf("%w: %w", common.ErrDependencyUnavailable, err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// GenerateEventID uses the same UUID format as persisted session identifiers.
func GenerateEventID(entropy io.Reader) (string, error) { return randomUUID(entropy) }
