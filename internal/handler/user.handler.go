package handler

import (
	"context"

	userfeature "github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pb/paginator"
	pbuser "github.com/siti-nabila/grpc-auth/pb/user"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/orm"
	ormdictionary "github.com/siti-nabila/orm/pkg/dictionary"
)

func (u *UserHandler) ListUsers(ctx context.Context, in *pbuser.ListUsersRequest) (*pbuser.ListUsersResponse, error) {
	opts, err := queryOptionsFromProto(in.GetQuery())
	if err != nil {
		return nil, helpers.HandleError(err)
	}

	feat := userfeature.NewUserService(ctx)
	page, err := feat.SearchUsers(domain.UserListRequest{
		Query:  opts,
		LastID: in.GetQuery().GetLastId(),
	})
	if err != nil {
		return nil, helpers.HandleError(err)
	}

	return listUsersResponseFromPage(page), nil
}

func queryOptionsFromProto(in *paginator.PageQuery) (orm.QueryOptions, error) {
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
		mode, err := searchModeFromProto(search.GetMode())
		if err != nil {
			return orm.QueryOptions{}, err
		}
		opts.Search = &orm.SearchQuery{
			Fields:  append([]string(nil), search.GetFields()...),
			Keyword: search.GetKeyword(),
			Mode:    mode,
		}
	}

	return opts, nil
}

func searchModeFromProto(mode paginator.SearchMode) (orm.SearchMode, error) {
	switch mode {
	case paginator.SearchMode_SEARCH_MODE_UNSPECIFIED:
		return "", nil
	case paginator.SearchMode_SEARCH_MODE_CONTAINS:
		return orm.SearchModeContains, nil
	case paginator.SearchMode_SEARCH_MODE_PREFIX:
		return orm.SearchModePrefix, nil
	case paginator.SearchMode_SEARCH_MODE_FULL_TEXT:
		return orm.SearchModeFullText, nil
	case paginator.SearchMode_SEARCH_MODE_TRIGRAM:
		return orm.SearchModeTrigram, nil
	case paginator.SearchMode_SEARCH_MODE_FULL_TEXT_TRIGRAM:
		return orm.SearchModeFullTextTrigram, nil
	default:
		return "", ormdictionary.ErrInvalidSearchMode
	}
}

func listUsersResponseFromPage(page orm.PageData[domain.UserSearchRow]) *pbuser.ListUsersResponse {
	items := make([]*pbuser.UserListItem, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, &pbuser.UserListItem{
			Email:   row.Email,
			Name:    row.Name,
			Address: row.Address,
			Phone:   row.Phone,
			Id:      int32(row.AuthID),
		})
	}

	return &pbuser.ListUsersResponse{
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
