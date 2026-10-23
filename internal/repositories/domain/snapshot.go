package domain

import (
	"bytes"
	"encoding/json"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"io"
)

// SafeSnapshot is an allowlist, never a raw request, credential or arbitrary map.
type SafeSnapshot struct {
	Version         int64    `json:"version,omitempty"`
	Status          string   `json:"status,omitempty"`
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Address         *string  `json:"address,omitempty"`
	Phone           *string  `json:"phone,omitempty"`
	CountryCode     *string  `json:"country_code,omitempty"`
	RoleIDs         []uint64 `json:"role_ids,omitempty"`
	PermissionCodes []string `json:"permission_codes,omitempty"`
	ApplicationID   uint64   `json:"application_id,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	ReasonCode      string   `json:"reason_code,omitempty"`
	NoOp            bool     `json:"no_op,omitempty"`
}

func (s SafeSnapshot) Validate() error {
	if s.Version < 0 || !boundedText(s.Status, 16, false) || (s.SessionID != "" && !canonicalUUID.MatchString(s.SessionID)) || (s.ReasonCode != "" && (!boundedText(s.ReasonCode, 100, false) || !reasonCodePattern.MatchString(s.ReasonCode))) {
		return common.ErrBadRequest
	}
	for _, field := range []struct {
		value *string
		max   int
	}{{s.Name, 100}, {s.Description, 100}, {s.Address, 250}, {s.Phone, 20}, {s.CountryCode, 4}} {
		if field.value != nil && !boundedText(*field.value, field.max, false) {
			return common.ErrBadRequest
		}
	}
	for _, id := range s.RoleIDs {
		if id == 0 {
			return common.ErrBadRequest
		}
	}
	for _, code := range s.PermissionCodes {
		if !boundedText(code, 150, true) {
			return common.ErrBadRequest
		}
	}
	return nil
}

func (s *SafeSnapshot) UnmarshalJSON(data []byte) error {
	type fields SafeSnapshot
	var value fields
	if err := decodeSafeJSON(data, &value); err != nil {
		return err
	}
	if err := SafeSnapshot(value).Validate(); err != nil {
		return err
	}
	*s = SafeSnapshot(value)
	return nil
}

func decodeSafeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return common.ErrBadRequest
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return common.ErrBadRequest
	}
	return nil
}
