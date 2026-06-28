package domain

import (
	"errors"
	"strconv"
	"strings"

	errorpackage "github.com/siti-nabila/error-package"
	"github.com/siti-nabila/orm/orm"
)

const UserListCursorField = "auth_id"

type (
	UserReader interface {
		SearchUsers(opts orm.QueryOptions) (orm.PageData[UserSearchRow], error)
	}

	UserListRequest struct {
		Query  orm.QueryOptions
		LastID string
	}

	UserSearchRow struct {
		AuthID  int64  `sql:"column:auth_id" json:"auth_id"`
		Email   string `sql:"column:email" json:"email"`
		Name    string `sql:"column:name" json:"name"`
		Address string `sql:"column:address" json:"address"`
		Phone   string `sql:"column:phone" json:"phone"`
	}

	UserListCursorOptions struct {
		Query      orm.QueryOptions
		GlobalPage int
		Limit      int
	}
)

func (UserSearchRow) TableName() string {
	return "profile p"
}

func ValidateUserListSortDesc(sorts []orm.SortField) (bool, error) {
	if len(sorts) == 0 {
		return false, nil
	}

	for _, sort := range sorts {
		field := strings.TrimSpace(sort.Field)
		if field == "" {
			continue
		}
		if field != UserListCursorField {
			return false, NewUserListValidationError("sort", "list users cursor pagination only supports auth_id sort")
		}
		return sort.Desc, nil
	}

	return false, nil
}

func NewUserListValidationError(field, message string) error {
	errs := errorpackage.Errors{}
	errs.Add(field, errors.New(message))
	return errs
}

func IsLastIDInsideUserListBatch(lastIDText, startCursorText, nextCursorText string, sortDesc bool) bool {
	lastID, err := strconv.ParseInt(strings.TrimSpace(lastIDText), 10, 64)
	if err != nil {
		return false
	}
	startCursor, err := strconv.ParseInt(strings.TrimSpace(startCursorText), 10, 64)
	if err != nil {
		return false
	}
	nextCursor, err := strconv.ParseInt(strings.TrimSpace(nextCursorText), 10, 64)
	if err != nil {
		return false
	}

	if sortDesc {
		return lastID < startCursor && lastID >= nextCursor
	}
	return lastID > startCursor && lastID <= nextCursor
}

func IsEmptyUserListLastID(lastIDText string) bool {
	lastIDText = strings.TrimSpace(lastIDText)
	return lastIDText == "" || lastIDText == "0"
}
