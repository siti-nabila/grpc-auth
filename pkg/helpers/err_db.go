package helpers

import (
	"errors"

	authdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
)

// HandleDBError maps infrastructure-specific database errors to errors from the
// shared API registry. Errors without a public mapping keep their original
// identity so callers can handle or redact them at the transport boundary.
func HandleDBError(err error) error {
	if err == nil {
		return nil
	}

	if databaseError, ok := errors.AsType[*normalizeerr.DBError](err); ok {
		switch databaseError.Kind {
		case normalizeerr.KindDuplicateRow:
			return authdictionary.ErrDataExists
		case normalizeerr.KindRowNotFound:
			return authdictionary.ErrNotFound
		default:
			return databaseError
		}
	}

	return err
}
