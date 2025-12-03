package domain

import (
	"database/sql"
	"reflect"
	"regexp"

	errorpackage "github.com/siti-nabila/error-package"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type (
	AuthWriter interface {
		UseTransaction(tx *sql.Tx)
		Begin() (*sql.Tx, error)

		Register(*AuthRequest) error
		RegisterTx(request *AuthRequest) (err error)
	}

	AuthReader interface {
		GetById(id uint64) (AuthResponse, error)
		GetByEmail(email string) (AuthResponse, error)
	}

	AuthRequest struct {
		Id       uint64 `sql:"id" json:"id"`
		Email    string `sql:"email" json:"email"`
		Password string `sql:"password" json:"password"`
	}

	AuthResponse struct {
		Id        uint64       `sql:"id" json:"id"`
		Email     string       `sql:"email" json:"email"`
		Password  string       `sql:"password" json:"password"`
		CreatedAt sql.NullTime `sql:"created_at" json:"created_at"`
		UpdatedAt sql.NullTime `sql:"updated_at" json:"updated_at"`
		DeletedAt sql.NullTime `sql:"deleted_at" json:"deleted_at"`
	}
)

var emailRegex = regexp.MustCompile(
	`^[A-Za-z0-9](?:[A-Za-z0-9._~\-]*[A-Za-z0-9])?@` +
		`[A-Za-z0-9](?:[A-Za-z0-9._~\-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9._~\-]*[A-Za-z0-9])?)+$`,
)

func (a *AuthRequest) Validate() error {
	errs := errorpackage.Errors{}
	if er := a.validateEmail(); er != nil {
		if sub, ok := er.(errorpackage.Errors); ok {
			errs.Merge(sub)
		} else {
			errs.Add(helpers.EmailJsonTag, er)
		}
	}
	if er := a.validatePassword(); er != nil {
		if sub, ok := er.(errorpackage.Errors); ok {
			errs.Merge(sub)
		} else {
			errs.Add(helpers.PasswordJsonTag, er)
		}
	}
	if errs.Empty() {
		return nil
	}

	return errs
}

func (a *AuthRequest) validateEmail() error {
	errs := errorpackage.Errors{}

	if a.Email == "" {
		errs.Add(helpers.EmailJsonTag, dictionary.ErrRequired)
	}
	if len(a.Email) < 6 {
		errs.Add(helpers.EmailJsonTag, dictionary.ErrMinLength(6))
	}
	if len(a.Email) > 50 {
		errs.Add(helpers.EmailJsonTag, dictionary.ErrMaxLength(50))
	}
	if !isValidEmail(a.Email) && a.Email != "" {
		errs.Add(helpers.EmailJsonTag, dictionary.ErrInvalidEmail)
	}
	if len(errs) != 0 {
		return errs
	}

	return nil
}
func (a *AuthRequest) validatePassword() error {
	errs := errorpackage.Errors{}

	if a.Password == "" {
		errs.Add(helpers.PasswordJsonTag, dictionary.ErrRequired)
	}
	if len(a.Password) < 6 {
		errs.Add(helpers.PasswordJsonTag, dictionary.ErrMinLength(6))
	}
	if len(a.Password) > 50 {
		errs.Add(helpers.PasswordJsonTag, dictionary.ErrMaxLength(50))
	}
	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (a *AuthRequest) GetJSONTags() map[string]string {
	t := reflect.TypeOf(a)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	tags := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		tags[field.Name] = jsonTag
	}
	return tags
}

func (a *AuthRequest) GetTableName() string {
	return "auth"
}

func isValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}
