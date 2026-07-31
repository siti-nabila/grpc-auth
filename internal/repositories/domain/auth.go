package domain

import (
	"database/sql"
	"reflect"
	"regexp"

	"github.com/siti-nabila/api-contracts/pkg/dictionary"
	"github.com/siti-nabila/grpc-auth/pb/user"
	appdictionary "github.com/siti-nabila/grpc-auth/pkg/dictionary"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	AuthWriterOld interface {
		UseTransaction(tx *sql.Tx)
		Begin() (*sql.Tx, error)

		Register(*AuthRequest) error
		RegisterTx(request *AuthRequest) (err error)
	}
	AuthWriter interface {
		UseTransaction(tx *orm.SqlTransactionAdapter)
		Begin() (*orm.SqlTransactionAdapter, error)

		Create(*AuthRequest) error
		// RegisterTx(request *AuthRequest) (err error)
	}

	AuthReader interface {
		GetById(id uint64) (Auth, error)
		GetByEmail(email string) (Auth, error)
	}

	AuthRequest struct {
		Id       uint64 `sql:"column:id;primaryKey" json:"id"`
		Email    string `sql:"column:email" json:"email"`
		Password string `sql:"column:password" json:"password"`
	}
	Auth struct {
		Id        uint64       `sql:"column:id;primaryKey" json:"id"`
		Email     string       `sql:"column:email" json:"email"`
		Password  string       `sql:"column:password" json:"password"`
		CreatedAt sql.NullTime `sql:"column:created_at" json:"created_at"`
		UpdatedAt sql.NullTime `sql:"column:updated_at" json:"updated_at"`
		DeletedAt sql.NullTime `sql:"column:deleted_at" json:"deleted_at"`
	}

	AuthResponse struct {
		Id        uint64       `sql:"column:id;primaryKey" json:"id"`
		Email     string       `sql:"column:email" json:"email"`
		Password  string       `sql:"column:password" json:"password"`
		CreatedAt sql.NullTime `sql:"column:created_at" json:"created_at"`
		UpdatedAt sql.NullTime `sql:"column:updated_at" json:"updated_at"`
		DeletedAt sql.NullTime `sql:"column:deleted_at" json:"deleted_at"`
	}
)

var emailRegex = regexp.MustCompile(
	`^[A-Za-z0-9](?:[A-Za-z0-9._~\-]*[A-Za-z0-9])?@` +
		`[A-Za-z0-9](?:[A-Za-z0-9._~\-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9._~\-]*[A-Za-z0-9])?)+$`,
)

func (a Auth) ToUserDataResponse() *user.UserResponse {
	return &user.UserResponse{
		Id:        a.Id,
		Email:     a.Email,
		CreatedAt: timestamppb.New(a.CreatedAt.Time),
	}

}

func (a *AuthRequest) Validate() error {
	errs := dictionary.FieldErrors{}
	if er := a.validateEmail(); er != nil {
		if sub, ok := er.(dictionary.FieldErrors); ok {
			errs.Merge(sub)
		} else {
			errs.Add(helpers.EmailJsonTag, er)
		}
	}
	if er := a.validatePassword(); er != nil {
		if sub, ok := er.(dictionary.FieldErrors); ok {
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
	errs := dictionary.FieldErrors{}

	if a.Email == "" {
		errs.Add(helpers.EmailJsonTag, appdictionary.ErrRequired)
	}
	if len(a.Email) < 6 {
		errs.Add(helpers.EmailJsonTag, appdictionary.ErrMinLength(6))
	}
	if len(a.Email) > 50 {
		errs.Add(helpers.EmailJsonTag, appdictionary.ErrMaxLength(50))
	}
	if !isValidEmail(a.Email) && a.Email != "" {
		errs.Add(helpers.EmailJsonTag, appdictionary.ErrInvalidEmail)
	}
	if len(errs) != 0 {
		return errs
	}

	return nil
}
func (a *AuthRequest) validatePassword() error {
	errs := dictionary.FieldErrors{}

	if a.Password == "" {
		errs.Add(helpers.PasswordJsonTag, appdictionary.ErrRequired)
	}
	if len(a.Password) < 6 {
		errs.Add(helpers.PasswordJsonTag, appdictionary.ErrMinLength(6))
	}
	if len(a.Password) > 50 {
		errs.Add(helpers.PasswordJsonTag, appdictionary.ErrMaxLength(50))
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

func (a *AuthRequest) TableName() string {
	return "auth"
}

func (a *Auth) TableName() string {
	return "auth"
}
func isValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}
