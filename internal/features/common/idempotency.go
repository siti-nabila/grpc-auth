package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

// Payload contains normalized business fields only. Presence distinguishes a
// supplied empty scalar from an omitted scalar. Transport metadata is excluded.
type FingerprintPayload struct {
	Email           string   `json:"email,omitempty"`
	Code            string   `json:"code,omitempty"`
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Address         *string  `json:"address,omitempty"`
	Phone           *string  `json:"phone,omitempty"`
	CountryCode     *string  `json:"country_code,omitempty"`
	Status          string   `json:"status,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	ApplicationID   uint64   `json:"application_id,omitempty"`
	ExpectedVersion int64    `json:"expected_version"`
	Presence        []string `json:"presence"`
	RoleIDs         []uint64 `json:"role_ids"`
	PermissionCodes []string `json:"permission_codes"`
}

type Fingerprinter struct {
	active string
	keys   map[string][]byte
}

var fingerprintVersion = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// Retain every key version referenced by unexpired records for the full 24h
// window. Configuration is injected by the future consumer, never read here.
// Missing retained keys fail closed as dependency unavailable, not conflict.
func NewFingerprinter(active string, keys map[string][]byte) (*Fingerprinter, error) {
	if !fingerprintVersion.MatchString(active) || len(keys[active]) < 32 {
		return nil, common.ErrBadRequest
	}
	copyKeys := make(map[string][]byte, len(keys))
	for version, key := range keys {
		if !fingerprintVersion.MatchString(version) || len(key) < 32 {
			return nil, common.ErrBadRequest
		}
		copyKeys[version] = append([]byte(nil), key...)
	}
	return &Fingerprinter{active: active, keys: copyKeys}, nil
}

func IdempotencyLockKey(scope domain.IdempotencyScope) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	data, _ := json.Marshal(scope)
	digest := sha256.Sum256(data)
	return "auth:idem:" + hex.EncodeToString(digest[:]), nil
}

func canonicalFingerprint(scope domain.IdempotencyScope, payload FingerprintPayload) ([]byte, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if payload.ExpectedVersion < 0 {
		return nil, common.ErrBadRequest
	}
	payload.Email = strings.ToLower(strings.TrimSpace(payload.Email))
	payload.Code = strings.ToLower(strings.TrimSpace(payload.Code))
	payload.RoleIDs = slices.Clone(payload.RoleIDs)
	slices.Sort(payload.RoleIDs)
	payload.RoleIDs = slices.Compact(payload.RoleIDs)
	if len(payload.RoleIDs) == 0 {
		payload.RoleIDs = nil
	}
	for _, id := range payload.RoleIDs {
		if id == 0 {
			return nil, common.ErrBadRequest
		}
	}
	payload.PermissionCodes = canonicalSet(payload.PermissionCodes)
	payload.Presence = canonicalSet(payload.Presence)
	return json.Marshal(struct {
		Target  string
		Payload FingerprintPayload
	}{scope.Target, payload})
}
func canonicalSet(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	values = slices.Clone(values)
	slices.Sort(values)
	return slices.Compact(values)
}

func Fingerprint(scope domain.IdempotencyScope, payload FingerprintPayload) ([]byte, error) {
	data, err := canonicalFingerprint(scope, payload)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	return []byte("sha256-v1:" + hex.EncodeToString(digest[:])), nil
}

func (f *Fingerprinter) PasswordFingerprint(scope domain.IdempotencyScope, payload FingerprintPayload, password []byte) ([]byte, error) {
	if f == nil || len(password) == 0 {
		return nil, common.ErrBadRequest
	}
	return f.withVersion(f.active, scope, payload, password)
}
func (f *Fingerprinter) withVersion(version string, scope domain.IdempotencyScope, payload FingerprintPayload, password []byte) ([]byte, error) {
	if f == nil || len(f.keys[version]) < 32 {
		return nil, common.ErrDependencyUnavailable
	}
	data, err := canonicalFingerprint(scope, payload)
	if err != nil {
		return nil, err
	}
	digest := hmac.New(sha256.New, f.keys[version])
	// JSON is self-delimiting; a NUL separates canonical data from opaque password.
	digest.Write(data)
	digest.Write([]byte{0})
	digest.Write(password)
	return []byte("hmac-v1:" + version + ":" + hex.EncodeToString(digest.Sum(nil))), nil
}

// Replay must be called after current identity/permission checks, before target
// expected-version checks. An expired record is a new command under the lock.
func Replay(record *domain.IdempotencyRecord, hash []byte, now time.Time) (*domain.SafeCommandResult, error) {
	if now.IsZero() || len(hash) == 0 {
		return nil, common.ErrBadRequest
	}
	if record == nil || !now.Before(record.ExpiresAt) {
		return nil, nil
	}
	if !hmac.Equal(record.RequestHash, hash) {
		return nil, user.ErrIdempotencyConflict
	}
	result, err := record.Result()
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (f *Fingerprinter) ReplayPassword(record *domain.IdempotencyRecord, scope domain.IdempotencyScope, payload FingerprintPayload, password []byte, now time.Time) (*domain.SafeCommandResult, error) {
	if now.IsZero() || len(password) == 0 {
		return nil, common.ErrBadRequest
	}
	if record == nil || !now.Before(record.ExpiresAt) {
		return nil, nil
	}
	parts := strings.Split(string(record.RequestHash), ":")
	if len(parts) != 3 || parts[0] != "hmac-v1" || !fingerprintVersion.MatchString(parts[1]) || len(parts[2]) != 64 {
		return nil, common.ErrInternalServerError
	}
	hash, err := f.withVersion(parts[1], scope, payload, password)
	if err != nil {
		return nil, err
	}
	return Replay(record, hash, now)
}
