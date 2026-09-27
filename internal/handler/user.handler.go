package handler

import (
	"context"
	"strings"
	"time"

	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	userfeature "github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const jakartaUTCOffsetSeconds = 7 * 60 * 60

func (u *UserHandler) ListUsers(ctx context.Context, in *userv1.ListUsersRequest) (*userv1.ListUsersResponse, error) {
	request, err := UserListRequestFromProto(in)
	if err != nil {
		return nil, err
	}

	feat := userfeature.NewUserService(ctx)
	page, err := feat.SearchUsers(request)
	if err != nil {
		return nil, err
	}

	return ListUsersResponseFromPage(page), nil
}

func UserListRequestFromProto(in *userv1.ListUsersRequest) (domain.UserListRequest, error) {
	if in == nil {
		return domain.UserListRequest{}, commondictionary.ErrBadRequest
	}

	opts, err := queryOptionsFromProto(in.GetQuery())
	if err != nil {
		return domain.UserListRequest{}, err
	}
	filter, err := userListFilterFromProto(in.GetFilter())
	if err != nil {
		return domain.UserListRequest{}, err
	}

	return domain.UserListRequest{
		Query:  opts,
		LastID: in.GetQuery().GetLastId(),
		Filter: filter,
	}, nil
}

func queryOptionsFromProto(in *paginatorv1.PageQuery) (orm.QueryOptions, error) {
	if in == nil {
		return orm.QueryOptions{}, nil
	}

	opts := orm.QueryOptions{
		Page:   int(in.GetPage()),
		Limit:  int(in.GetLimit()),
		Select: append([]string(nil), in.GetFields()...),
	}

	for _, sort := range in.GetSort() {
		if sort == nil {
			continue
		}
		opts.Sort = append(opts.Sort, orm.SortField{
			Field: sort.GetField(),
			Desc:  sort.GetDesc(),
		})
	}

	if search := in.GetSearch(); search != nil {
		if _, err := searchModeFromProto(search.GetMode()); err != nil {
			return orm.QueryOptions{}, err
		}
		if keyword := strings.TrimSpace(search.GetKeyword()); keyword != "" {
			opts.Search = &orm.SearchQuery{
				Fields:  []string{domain.UserListSearchField},
				Keyword: keyword,
				Mode:    orm.SearchModeFullTextTrigram,
			}
		}
	}

	return opts, nil
}

func userListFilterFromProto(in *userv1.UserFilter) (domain.UserListFilter, error) {
	if in == nil {
		return domain.UserListFilter{}, nil
	}

	createdFrom, err := timestampFromProto(in.GetCreatedFrom())
	if err != nil {
		return domain.UserListFilter{}, err
	}
	createdTo, err := timestampFromProto(in.GetCreatedTo())
	if err != nil {
		return domain.UserListFilter{}, err
	}
	if createdFrom != nil && createdTo != nil && !createdFrom.Before(*createdTo) {
		return domain.UserListFilter{}, commondictionary.ErrBadRequest
	}

	roleCodes := make([]uint64, 0, len(in.GetRoleCodes()))
	seen := make(map[uint64]struct{}, len(in.GetRoleCodes()))
	for _, roleCode := range in.GetRoleCodes() {
		if roleCode == 0 {
			return domain.UserListFilter{}, commondictionary.ErrBadRequest
		}
		if _, exists := seen[roleCode]; exists {
			continue
		}
		seen[roleCode] = struct{}{}
		roleCodes = append(roleCodes, roleCode)
	}

	return domain.UserListFilter{
		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,
		RoleCodes:   roleCodes,
	}, nil
}

func timestampFromProto(value *timestamppb.Timestamp) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	if err := value.CheckValid(); err != nil {
		return nil, commondictionary.ErrBadRequest
	}
	timestamp := value.AsTime().In(
		time.FixedZone("Asia/Jakarta", jakartaUTCOffsetSeconds),
	)
	return &timestamp, nil
}

func searchModeFromProto(mode paginatorv1.SearchMode) (orm.SearchMode, error) {
	switch mode {
	case paginatorv1.SearchMode_SEARCH_MODE_UNSPECIFIED:
		return "", nil
	case paginatorv1.SearchMode_SEARCH_MODE_CONTAINS:
		return orm.SearchModeContains, nil
	case paginatorv1.SearchMode_SEARCH_MODE_PREFIX:
		return orm.SearchModePrefix, nil
	case paginatorv1.SearchMode_SEARCH_MODE_FULL_TEXT:
		return orm.SearchModeFullText, nil
	case paginatorv1.SearchMode_SEARCH_MODE_TRIGRAM:
		return orm.SearchModeTrigram, nil
	case paginatorv1.SearchMode_SEARCH_MODE_FULL_TEXT_TRIGRAM:
		return orm.SearchModeFullTextTrigram, nil
	default:
		return "", commondictionary.ErrBadRequest
	}
}

func ListUsersResponseFromPage(page orm.PageData[domain.UserSearchRow]) *userv1.ListUsersResponse {
	items := make([]*userv1.UserListItem, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, &userv1.UserListItem{
			Email:   row.Email,
			Name:    row.Name,
			Address: row.Address,
			Phone:   row.Phone,
			Id:      int32(row.AuthID),
		})
	}

	return &userv1.ListUsersResponse{
		Items:      items,
		Total:      int32(page.Total),
		Page:       int32(page.Page),
		Limit:      int32(page.Limit),
		TotalPages: int32(page.TotalPages),
		HasNext:    page.HasNext,
		HasPrev:    page.HasPrev,
		NextCursor: page.NextCursor,
	}
}
