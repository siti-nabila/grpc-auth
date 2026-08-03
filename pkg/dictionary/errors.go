package dictionary

import (
	"errors"

	authdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
)

var (
	ErrDataExists          = authdictionary.ErrDataExists
	ErrPasswordMismatch    = authdictionary.ErrPasswordMismatch
	ErrRequired            = authdictionary.ErrRequired
	ErrBadRequest          = commondictionary.ErrBadRequest
	ErrInvalidEmail        = authdictionary.ErrInvalidEmail
	ErrNotFound            = authdictionary.ErrNotFound
	ErrInternalServerError = commondictionary.ErrInternalServerError
)

func HandleDBError(err error) error {
	if err == nil {
		return nil
	}

	if databaseError, ok := errors.AsType[*normalizeerr.DBError](err); ok {
		switch databaseError.Kind {
		case normalizeerr.KindDuplicateRow:
			return ErrDataExists
		case normalizeerr.KindRowNotFound:
			return ErrNotFound
		default:
			return databaseError
		}
	}

	return err
}
