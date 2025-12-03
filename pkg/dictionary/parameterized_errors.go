package dictionary

import (
	_ "embed"

	errorpackage "github.com/siti-nabila/error-package"
)

var (
	// errMinLength error
	// ErrMaxLength error

	//go:embed parameterized_err_list.yaml
	paramsErrorList []byte
)

func init() {
	errPack = errorpackage.NewErrYamlPackage()
	errPack.LoadBytes(paramsErrorList)
	// ErrMinLength = errPack.New("err_min_length")
	// ErrMaxLength = errPack.New("err_max_length")

}

func ErrMinLength(length int) error {
	return errPack.Newf("err_min_length", length)
}

func ErrMaxLength(length int) error {
	return errPack.Newf("err_max_length", length)
}
func ErrGeneratingToken(err error) error {
	return errPack.Newf("err_generate_token", err.Error())
}
