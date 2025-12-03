package dictionary

import (
	_ "embed"

	errorpackage "github.com/siti-nabila/error-package"
)

var (
	errPack             errorpackage.DictionaryPack
	ErrDuplicateKey     error
	ErrPasswordMismatch error
	ErrRequired         error
	ErrBadRequest       error
	ErrInvalidEmail     error

	//go:embed err_list.yaml
	errorList []byte
)

func init() {
	errPack = errorpackage.NewErrYamlPackage()
	errPack.LoadBytes(errorList)
	ErrDuplicateKey = errPack.New("err_duplicate_key")
	ErrPasswordMismatch = errPack.New("err_password_mismatch")
	ErrRequired = errPack.New("err_required")
	ErrBadRequest = errPack.New("err_bad_request")
	ErrInvalidEmail = errPack.New("err_invalid_email")

}
