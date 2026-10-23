package domain

import (
	"database/sql"
	"regexp"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
)

var countryCodePattern = regexp.MustCompile(`^\+[1-9][0-9]{0,2}$`)

// ValidateCountryCode accepts an empty nullable value or a telephone calling code.
func ValidateCountryCode(value string) error {
	if value != "" && !countryCodePattern.MatchString(value) {
		return common.ErrBadRequest
	}
	return nil
}

// NullableCountryCode maps omitted or cleared input to SQL NULL without a default.
func NullableCountryCode(value *string) sql.NullString {
	if value == nil || *value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
