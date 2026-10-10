package profile

import (
	"unicode/utf8"

	"github.com/siti-nabila/api-contracts/pkg/dictionary"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

func selectedFields(req domain.ProfileUpdate) (map[string]bool, error) {
	fields := map[string]bool{"name": !req.MaskPresent, "address": !req.MaskPresent, "phone": !req.MaskPresent}
	for _, field := range req.Mask {
		if _, ok := fields[field]; !ok {
			return nil, common.ErrBadRequest
		}
		fields[field] = true
	}
	errs := dictionary.FieldErrors{}
	for _, field := range []struct {
		name, value string
		min, max    int
	}{{"name", req.Name, 3, 100}, {"address", req.Address, 0, 250}, {"phone", req.Phone, 0, 20}} {
		if !fields[field.name] {
			continue
		}
		n := utf8.RuneCountInString(field.value)
		if !utf8.ValidString(field.value) {
			errs.Add(field.name, common.ErrBadRequest)
		} else if n < field.min {
			errs.Add(field.name, auth.ErrMinLength(field.min))
		} else if n > field.max {
			errs.Add(field.name, auth.ErrMaxLength(field.max))
		}
	}
	if !errs.Empty() {
		return nil, errs
	}
	return fields, nil
}

func replaceFields(current domain.Profile, req domain.ProfileUpdate, fields map[string]bool) domain.Profile {
	if fields["name"] {
		current.Name = req.Name
	}
	if fields["address"] {
		current.Address = req.Address
	}
	if fields["phone"] {
		current.Phone = req.Phone
	}
	return current
}
