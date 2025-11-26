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

func (a *AuthRequest) GetTableName() string {
	return "auth"
}
