package user

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/pagination"
	"math"
	"strings"
)

const (
	userListMaxLimit     = pagination.MaxLimit
	userListDefaultLimit = pagination.DefaultLimit
)

func (u *userService) SearchUsers(req domain.UserListRequest) (orm.PageData[domain.UserSearchRow], error) {
	opts := req.Query
	fail := func(err error) (orm.PageData[domain.UserSearchRow], error) {
		return orm.PageData[domain.UserSearchRow]{}, err
	}
	sorts, err := domain.NormalizeUserListSort(opts.Sort)
	if err != nil {
		return fail(err)
	}
	opts.Sort = sorts
	filter, err := domain.NormalizeUserListFilter(req.Filter)
	if err != nil {
		return fail(err)
	}
	if opts.Page < 0 || opts.Page > math.MaxInt32 || opts.Limit < 0 || opts.SearchAnd != nil || len(opts.Filters) > 0 {
		return fail(common.ErrBadRequest)
	}
	if opts.Page == 0 {
		opts.Page = 1
	}
	if opts.Limit == 0 {
		opts.Limit = userListDefaultLimit
	}
	if opts.Limit > userListMaxLimit {
		opts.Limit = userListMaxLimit
	}
	for _, field := range opts.Select {
		if domain.UserSortKind(field) == "" && field != domain.UserListCursorField {
			return fail(common.ErrBadRequest)
		}
	}
	if opts.Search != nil {
		q := *opts.Search
		if len(q.Fields) != 1 || q.Fields[0] != domain.UserListSearchField || (q.Mode != orm.SearchModeFullText && q.Mode != orm.SearchModeFullTextTrigram) {
			return fail(common.ErrBadRequest)
		}
		q.Keyword = strings.Join(strings.Fields(q.Keyword), " ")
		q.Fields = []string{domain.UserListSearchField}
		opts.Search = &q
		if q.Keyword == "" {
			opts.Search = nil
		}
	}
	cursor, err := domain.ParseUserListCursor(strings.TrimSpace(req.LastID), opts, filter)
	if err != nil {
		return fail(err)
	}
	opts.InMemoryOffset = &orm.InMemoryOffsetOptions{MaxLimit: userListMaxLimit, Cursor: orm.Cursor{Field: domain.UserListCursorField, Value: cursor}}
	page, err := u.userReader.SearchUsers(opts, filter)
	if err != nil {
		return fail(err)
	}
	page.Page = opts.Page
	page.Limit = opts.Limit
	page.HasPrev = opts.Page > 1
	if len(opts.Select) > 0 {
		for i := range page.Items {
			page.Items[i] = projectUser(page.Items[i], opts.Select)
		}
	}
	return page, nil
}
func projectUser(row domain.UserSearchRow, fields []string) domain.UserSearchRow {
	var out domain.UserSearchRow
	for _, field := range fields {
		switch field {
		case domain.UserSortID, domain.UserListCursorField:
			out.AuthID = row.AuthID
		case domain.UserSortName:
			out.Name = row.Name
		case domain.UserSortEmail:
			out.Email = row.Email
		case domain.UserSortAddress:
			out.Address = row.Address
		case domain.UserSortPhone:
			out.Phone = row.Phone
		case domain.UserSortStatus:
			out.Status = row.Status
		case domain.UserSortVersion:
			out.Version = row.Version
		case domain.UserSortCreatedAt:
			out.CreatedAt = row.CreatedAt
		case domain.UserSortRoles:
			out.Roles = row.Roles
		case domain.UserSortApplicationAccess:
			out.ApplicationAccess = row.ApplicationAccess
		}
	}
	return out
}
