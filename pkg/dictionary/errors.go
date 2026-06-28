package dictionary

import (
	_ "embed"
	"errors"

	errorpackage "github.com/siti-nabila/error-package"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
)

var (
	errPack                errorpackage.DictionaryPack
	ErrDataExists          error
	ErrPasswordMismatch    error
	ErrRequired            error
	ErrBadRequest          error
	ErrInvalidEmail        error
	ErrNotFound            error
	ErrInternalServerError error

	//go:embed err_list.yaml
	errorList []byte
)

func init() {
	errPack = errorpackage.NewErrYamlPackage()
	errPack.LoadBytes(errorList)
	ErrDataExists = errPack.New("err_already_exists")
	ErrPasswordMismatch = errPack.New("err_password_mismatch")
	ErrRequired = errPack.New("err_required")
	ErrBadRequest = errPack.New("err_bad_request")
	ErrNotFound = errPack.New("err_not_found")
	ErrInternalServerError = errPack.New("err_internal_server_error")

}

func HandleDBError(err error) error {
	if err == nil {
		return nil
	}

	var er *normalizeerr.DBError
	if ok := errors.As(err, &er); ok {
		switch er.Kind {
		case normalizeerr.KindDuplicateRow:
			return ErrDataExists
		case normalizeerr.KindRowNotFound:
			return ErrNotFound
		default:
			return er
		}
	}

	return err
}
