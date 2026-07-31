package user

import (
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/paginator"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/pagination"
)

const (
	userListMaxLimit     = pagination.MaxLimit
	userListDefaultLimit = pagination.DefaultLimit
)

func (u *userService) SearchUsers(req domain.UserListRequest) (orm.PageData[domain.UserSearchRow], error) {
	opts := req.Query
	sortDesc, err := domain.ValidateUserListSortDesc(opts.Sort)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	opts.Sort = nil
	pager, err := paginator.Build(opts, paginator.Config{
		Mode:         paginator.ModeCursor,
		DefaultLimit: userListDefaultLimit,
		MaxLimit:     userListMaxLimit,
		BatchLimit:   userListMaxLimit,
		Cursor: &paginator.CursorConfig{
			Field:        domain.UserListCursorField,
			Value:        req.LastID,
			RequestField: "last_id",
			Parser:       paginator.PositiveInt64Cursor,
		},
		DefaultSort: []orm.SortField{
			{
				Field: domain.UserListCursorField,
				Desc:  sortDesc,
			},
		},
	})
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	pageData, err := u.userReader.SearchUsers(pager.Options)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	pageData.Page = pager.Page
	pageData.Limit = pager.Limit
	pageData.HasPrev = pager.Page > 1
	return pageData, nil
}
