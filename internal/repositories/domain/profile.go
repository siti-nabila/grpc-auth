package domain

import (
	"database/sql"
	"errors"
	"reflect"
)

type (
	ProfileWriter interface {
		UseTransaction(tx *sql.Tx)
		Begin() (*sql.Tx, error)

		// CreateProfile(*AuthRequest) error
		CreateProfileTx(request *ProfileRequest) (err error)
	}

	ProfileRequest struct {
		Id      uint64 `sql:"id" json:"id"`
		UserId  uint64 `sql:"user_id" json:"user_id"`
		Name    string `sql:"name" json:"name"`
		Address string `sql:"address" json:"address"`
		Phone   string `sql:"phone" json:"phone"`
	}
)

func (p *ProfileRequest) Validate() (errs map[string]error) {
	errs = make(map[string]error, 0)
	jsonTags := p.GetJSONTags()
	// tag := jsonTags[a.Email]
	if p.Name == "" {
		errs[jsonTags[p.Name]] = errors.New("name is required")
	}
	if len(p.Name) > 100 {
		errs[jsonTags[p.Name]] = errors.New("max length for name is 100")
	}
	if len(p.Name) < 3 {
		errs[jsonTags[p.Name]] = errors.New("min length for name is 3")
	}
	if len(errs) != 0 {
		return errs
	}
	return nil
}

func (p *ProfileRequest) GetJSONTags() map[string]string {
	t := reflect.TypeOf(p)
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

func (p *ProfileRequest) GetTableName() string {
	return "profile"
}
