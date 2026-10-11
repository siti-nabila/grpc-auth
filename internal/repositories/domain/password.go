package domain

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
)

type ChangePasswordInput struct {
	CurrentPassword string `json:"-"`
	NewPassword     string `json:"-"`
}

func (r ChangePasswordInput) Validate() error {
	errs := dictionary.FieldErrors{}
	for _, field := range []struct{ name, value string }{{"current_password", r.CurrentPassword}, {"new_password", r.NewPassword}} {
		if field.value == "" {
			errs.Add(field.name, auth.ErrRequired)
		}
		if len(field.value) < 6 {
			errs.Add(field.name, auth.ErrMinLength(6))
		}
		if len(field.value) > 50 {
			errs.Add(field.name, auth.ErrMaxLength(50))
		}
	}
	if errs.Empty() {
		return nil
	}
	return errs
}
