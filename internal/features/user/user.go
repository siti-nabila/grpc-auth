package user

import (
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/pagination"
)

const (
	userListMaxLimit     = pagination.MaxLimit
	userListDefaultLimit = pagination.DefaultLimit
	userListMaxAuthID    = "2147483647"
)

func (u *userService) SearchUsers(req domain.UserListRequest) (orm.PageData[domain.UserSearchRow], error) {
	opts := req.Query
	page, limit, err := normalizeUserListPagination(opts.Page, opts.Limit)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	batchIndex, pageInBatch, _, err := calculateUserListBatch(page, limit, userListMaxLimit)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	sortDesc, err := domain.ValidateUserListSortDesc(opts.Sort)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	startCursor, ok, err := u.resolveUserListBatchStartCursor(opts, batchIndex, limit, sortDesc)
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}
	if !ok {
		return emptyUserSearchPage(page, limit), nil
	}

	pageData, err := u.userReader.SearchUsers(userListCursorQueryOptions(opts, pageInBatch, limit, startCursor, sortDesc))
	if err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}

	if page == 1 && domain.IsEmptyUserListLastID(req.LastID) {
		pageData.Page = page
		pageData.Limit = limit
		pageData.HasPrev = false
		return pageData, nil
	}
	if pageData.NextCursor != "" && !domain.IsLastIDInsideUserListBatch(req.LastID, startCursor, pageData.NextCursor, sortDesc) {
		return emptyUserSearchPage(page, limit), nil
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

func calculateUserListBatch(page, limit, maxLimit int) (int, int, int, error) {
	batchSize := maxLimit / limit
	if batchSize <= 0 {
		return 0, 0, 0, domain.NewUserListValidationError("limit", "invalid pagination limit")
	}

	batchIndex := (page - 1) / batchSize
	pageInBatch := ((page - 1) % batchSize) + 1
	return batchIndex, pageInBatch, batchSize, nil
}

func (u *userService) resolveUserListBatchStartCursor(opts orm.QueryOptions, targetBatchIndex, limit int, sortDesc bool) (string, bool, error) {
	cursorValue := initialUserListCursor(sortDesc)
	if targetBatchIndex == 0 {
		return cursorValue, true, nil
	}

	for batchIndex := 0; batchIndex < targetBatchIndex; batchIndex++ {
		pageData, err := u.userReader.SearchUsers(userListCursorQueryOptions(opts, 1, limit, cursorValue, sortDesc))
		if err != nil {
			return "", false, err
		}
		if pageData.NextCursor == "" {
			return "", false, nil
		}
		cursorValue = pageData.NextCursor
	}

	return cursorValue, true, nil
}

func initialUserListCursor(sortDesc bool) string {
	if sortDesc {
		return userListMaxAuthID
	}
	return "0"
}

func userListCursorQueryOptions(opts orm.QueryOptions, page, limit int, cursorValue string, sortDesc bool) orm.QueryOptions {
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
