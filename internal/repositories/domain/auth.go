package domain

import (
	"database/sql"
	"errors"
	"reflect"
)

type (
	AuthWriter interface {
		UseTransaction(tx *sql.Tx)
		Begin() (*sql.Tx, error)

		Register(*AuthRequest) error
		RegisterTx(request *AuthRequest) (err error)
	}

	AuthRequest struct {
		Id       uint64 `sql:"id" json:"id"`
		Email    string `sql:"email" json:"email"`
		Password string `sql:"password" json:"password"`
	}
)

func (a *AuthRequest) Validate() (errs map[string]error) {
	errs = make(map[string]error, 0)
	jsonTags := a.GetJSONTags()
	// tag := jsonTags[a.Email]
	if a.Email == "" {
		errs[jsonTags[a.Email]] = errors.New("email is required")
	}
	if a.Password == "" {
		errs[jsonTags[a.Password]] = errors.New("password is required")
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
