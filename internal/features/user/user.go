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
	opts, filter, err := normalizeUserQuery(req)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}
	page, err := u.userReader.SearchUsers(opts, filter)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
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
		case "country_code":
			out.CountryCode = row.CountryCode
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

func normalizeUserQuery(req domain.UserListRequest) (orm.QueryOptions, domain.UserListFilter, error) {
	opts := req.Query
	var filter domain.UserListFilter
	sorts, err := domain.NormalizeUserListSort(opts.Sort)
	if err != nil {
		return opts, filter, err
	}
	opts.Sort = sorts
	filter, err = domain.NormalizeUserListFilter(req.Filter)
	if err != nil {
		return opts, filter, err
	}
	if err = normalizeUserOptions(&opts); err != nil {
		return opts, filter, err
	}
	if err = normalizeUserSearch(&opts); err != nil {
		return opts, filter, err
	}
	cursor, err := domain.ParseUserListCursor(strings.TrimSpace(req.LastID), opts, filter)
	if err != nil {
		return opts, filter, err
	}
	opts.InMemoryOffset = &orm.InMemoryOffsetOptions{MaxLimit: userListMaxLimit, Cursor: orm.Cursor{Field: domain.UserListCursorField, Value: cursor}}
	return opts, filter, nil
}

func normalizeUserOptions(opts *orm.QueryOptions) error {
	if opts.Page < 0 || opts.Page > math.MaxInt32 || opts.Limit < 0 || opts.SearchAnd != nil || len(opts.Filters) > 0 {
		return common.ErrBadRequest
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
		if domain.UserSortKind(field) == "" && field != domain.UserListCursorField && field != "country_code" {
			return common.ErrBadRequest
		}
	}
	return nil
}

func normalizeUserSearch(opts *orm.QueryOptions) error {
	if opts.Search != nil {
		q := *opts.Search
		if len(q.Fields) != 1 || q.Fields[0] != domain.UserListSearchField || (q.Mode != orm.SearchModeFullText && q.Mode != orm.SearchModeFullTextTrigram) {
			return common.ErrBadRequest
		}
		q.Keyword = strings.Join(strings.Fields(q.Keyword), " ")
		q.Fields = []string{domain.UserListSearchField}
		opts.Search = &q
		if q.Keyword == "" {
			opts.Search = nil
		}
	}
	return nil
}
