package user

import (
	"strconv"
	"strings"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/pagination"
)

const (
	userListMaxLimit     = pagination.MaxLimit
	userListDefaultLimit = pagination.DefaultLimit
)

func (u *userService) SearchUsers(req domain.UserListRequest) (orm.PageData[domain.UserSearchRow], error) {
	opts := req.Query
	page, limit, err := normalizeUserListPagination(opts.Page, opts.Limit)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	sortDesc, err := domain.ValidateUserListSortDesc(opts.Sort)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	cursorText := strings.TrimSpace(req.LastID)
	cursorValue, err := normalizeUserListCursorValue(cursorText)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	pageData, err := u.userReader.SearchUsers(userListCursorQueryOptions(opts, page, limit, cursorValue, sortDesc))
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	pageData.Page = page
	pageData.Limit = limit
	pageData.HasPrev = page > 1
	return pageData, nil
}

func normalizeUserListPagination(page, limit int) (int, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = userListDefaultLimit
	}
	if limit > userListMaxLimit {
		limit = userListMaxLimit
	}
	return page, limit, nil
}

func normalizeUserListCursorValue(lastIDText string) (any, error) {
	lastIDText = strings.TrimSpace(lastIDText)
	if domain.IsEmptyUserListLastID(lastIDText) {
		return "", nil
	}

	lastID, err := strconv.ParseInt(lastIDText, 10, 64)
	if err != nil || lastID < 1 {
		return nil, domain.NewUserListValidationError("last_id", "list users cursor must be a positive auth_id")
	}
	return lastID, nil
}

func userListCursorQueryOptions(opts orm.QueryOptions, page, limit int, cursorValue any, sortDesc bool) orm.QueryOptions {
	opts.Page = page
	opts.Limit = limit
	opts.InMemoryOffset = &orm.InMemoryOffsetOptions{
		Cursor: orm.Cursor{
			Field: domain.UserListCursorField,
			Value: cursorValue,
		},
		MaxLimit: userListMaxLimit,
	}
	opts.Sort = []orm.SortField{
		{
			Field: domain.UserListCursorField,
			Desc:  sortDesc,
		},
	}
	return opts
}

func emptyUserSearchPage(page, limit int) orm.PageData[domain.UserSearchRow] {
	if page < 1 {
		page = 1
	}
	return orm.PageData[domain.UserSearchRow]{
		Items:      []domain.UserSearchRow{},
		Total:      0,
		Page:       page,
		Limit:      limit,
		TotalPages: 0,
		HasNext:    false,
		HasPrev:    page > 1,
	}
}
