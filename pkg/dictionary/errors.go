package dictionary

import (
	_ "embed"

	errorpackage "github.com/siti-nabila/error-package"
)

var (
	errPack         errorpackage.DictionaryPack
	ErrDuplicateKey error

	//go:embed err_list.yaml
	errorList []byte
)

func init() {
	errPack = errorpackage.NewErrYamlPackage()
	errPack.LoadBytes(errorList)
	ErrDuplicateKey = errPack.New("err_duplicate_key")
}
