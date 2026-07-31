package dictionary

import authdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/auth"

func ErrMinLength(length int) error {
	return authdictionary.ErrMinLength(length)
}

func ErrMaxLength(length int) error {
	return authdictionary.ErrMaxLength(length)
}

func ErrGeneratingToken(error) error {
	return authdictionary.ErrGeneratingToken
}
