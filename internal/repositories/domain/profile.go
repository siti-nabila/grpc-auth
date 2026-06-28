package domain

import (
	"database/sql"
	"errors"
	"reflect"

	"github.com/siti-nabila/grpc-auth/pb/profile"
	"github.com/siti-nabila/orm/orm"
)

type (
	ProfileWriterOld interface {
		UseTransaction(tx *sql.Tx)
		Begin() (*sql.Tx, error)

		// CreateProfile(*AuthRequest) error
		CreateProfileTx(request *ProfileRequest) (err error)
	}

	ProfileReader interface {
		GetByUserId(userId uint64) (Profile, error)
	}

	ProfileWriter interface {
		UseTransaction(tx *orm.SqlTransactionAdapter)
		Begin() (*orm.SqlTransactionAdapter, error)

		// CreateProfile(*AuthRequest) error
		Create(request *ProfileRequest) (err error)
		Update(request *UpdateProfileRequest) (err error)
	}

	Profile struct {
		Id      uint64 `sql:"column:id;primaryKey" json:"id"`
		UserId  uint64 `sql:"column:user_id" json:"user_id"`
		Name    string `sql:"column:name" json:"name"`
		Address string `sql:"column:address" json:"address"`
		Phone   string `sql:"column:phone" json:"phone"`
	}

	ProfileRequest struct {
		Id      uint64 `sql:"column:id;primaryKey" json:"id"`
		UserId  uint64 `sql:"column:user_id" json:"user_id"`
		Name    string `sql:"column:name" json:"name"`
		Address string `sql:"column:address" json:"address"`
		Phone   string `sql:"column:phone" json:"phone"`
	}

	UpdateProfileRequest struct {
		Id      uint64 `sql:"column:id;primaryKey" json:"id"`
		Name    string `sql:"column:name" json:"name"`
		Address string `sql:"column:address" json:"address"`
		Phone   string `sql:"column:phone" json:"phone"`
	}
)

func (UpdateProfileRequest) TableName() string {
	return "profile"
}
func (Profile) TableName() string {
	return "profile"
}

func (p Profile) ToProfileResponse() *profile.Profile {
	return &profile.Profile{
		Id:      p.Id,
		UserId:  p.UserId,
		Name:    p.Name,
		Address: p.Address,
		Phone:   p.Phone,
	}
}

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

func (p *ProfileRequest) TableName() string {
	return "profile"
}
