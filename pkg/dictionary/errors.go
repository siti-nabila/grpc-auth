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
	ErrMinLength        error
	ErrMaxLength        error
	ErrBadRequest       error

	//go:embed err_list.yaml
	errorList []byte
)

func init() {
	errPack = errorpackage.NewErrYamlPackage()
	errPack.LoadBytes(errorList)
	ErrDuplicateKey = errPack.New("err_duplicate_key")
	ErrPasswordMismatch = errPack.New("err_password_mismatch")
	ErrRequired = errPack.New("err_required")
	ErrMinLength = errPack.New("err_min_length")
	ErrMaxLength = errPack.New("err_max_length")
	ErrBadRequest = errPack.New("err_bad_request")

}
