package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{
			Name: "maps next cursor into list users response",
			Run:  mapNextCursor,
		},
		{
			Name: "uses shared common registry error for invalid search mode",
			Run:  rejectInvalidSearchMode,
		},
		{
			Name: "maps list filters and applies backend-owned keyword search policy",
			Run:  mapListFiltersAndKeyword,
		},
		{
			Name: "rejects invalid filter timestamp",
			Run:  rejectInvalidFilterTimestamp,
		},
		{
			Name: "rejects inverted created date range",
			Run:  rejectInvertedCreatedRange,
		},
		{
			Name: "rejects zero role code",
			Run:  rejectZeroRoleCode,
		},
	}
}

func mapListFiltersAndKeyword(t *testing.T) {
	createdFrom := time.Date(2026, time.July, 31, 17, 0, 0, 0, time.UTC)
	createdTo := time.Date(2026, time.August, 31, 17, 0, 0, 0, time.UTC)
	request := &userv1.ListUsersRequest{
		Query: &paginatorv1.PageQuery{
			Page:   2,
			Limit:  20,
			LastId: "17",
			Search: &paginatorv1.Search{
				Fields:  []string{"client-controlled-field"},
				Keyword: "  blek  ",
				Mode:    paginatorv1.SearchMode_SEARCH_MODE_PREFIX,
			},
		},
		Filter: &userv1.UserFilter{
			CreatedFrom: timestamppb.New(createdFrom),
			CreatedTo:   timestamppb.New(createdTo),
			RoleCodes:   []uint64{1, 2, 1},
		},
	}

	actual, err := handler.UserListRequestFromProto(request)

	if err != nil {
		t.Fatalf("UserListRequestFromProto() error = %v", err)
	}
	if actual.Query.Page != 2 || actual.Query.Limit != 20 || actual.LastID != "17" {
		t.Errorf("pagination mapping = %#v, want page=2 limit=20 last_id=17", actual)
	}
	if actual.Query.Search == nil {
		t.Fatal("search = nil, want backend-owned keyword search")
	}
	if !reflect.DeepEqual(actual.Query.Search.Fields, []string{domain.UserListSearchField}) ||
		actual.Query.Search.Keyword != "blek" ||
		actual.Query.Search.Mode != orm.SearchModeFullTextTrigram {
		t.Errorf("search = %#v, want keyword field and full-text-trigram mode", actual.Query.Search)
	}
	if actual.Filter.CreatedFrom == nil || !actual.Filter.CreatedFrom.Equal(createdFrom) {
		t.Errorf("created_from = %v, want %v", actual.Filter.CreatedFrom, createdFrom)
	}
	if actual.Filter.CreatedTo == nil || !actual.Filter.CreatedTo.Equal(createdTo) {
		t.Errorf("created_to = %v, want %v", actual.Filter.CreatedTo, createdTo)
	}
	if !reflect.DeepEqual(actual.Filter.RoleCodes, []uint64{1, 2}) {
		t.Errorf("role codes = %v, want [1 2]", actual.Filter.RoleCodes)
	}
}

func rejectInvalidFilterTimestamp(t *testing.T) {
	assertBadListRequest(t, &userv1.ListUsersRequest{
		Filter: &userv1.UserFilter{
			CreatedFrom: &timestamppb.Timestamp{Seconds: 253402300800},
		},
	})
}

func rejectInvertedCreatedRange(t *testing.T) {
	assertBadListRequest(t, &userv1.ListUsersRequest{
		Filter: &userv1.UserFilter{
			CreatedFrom: timestamppb.New(time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC)),
			CreatedTo:   timestamppb.New(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)),
		},
	})
}

func rejectZeroRoleCode(t *testing.T) {
	assertBadListRequest(t, &userv1.ListUsersRequest{
		Filter: &userv1.UserFilter{RoleCodes: []uint64{0}},
	})
}

func assertBadListRequest(t *testing.T, request *userv1.ListUsersRequest) {
	t.Helper()

	_, err := handler.UserListRequestFromProto(request)
	if !errors.Is(err, commondictionary.ErrBadRequest) {
		t.Fatalf("UserListRequestFromProto() error = %v, want common ErrBadRequest", err)
	}
}

func rejectInvalidSearchMode(t *testing.T) {
	request := &userv1.ListUsersRequest{
		Query: &paginatorv1.PageQuery{
			Search: &paginatorv1.Search{
				Mode: paginatorv1.SearchMode(999),
			},
		},
	}

	_, err := (&handler.UserHandler{}).ListUsers(context.Background(), request)

	if !errors.Is(err, commondictionary.ErrBadRequest) {
		t.Fatalf("ListUsers() error = %v, want common registry ErrBadRequest", err)
	}
}

func mapNextCursor(t *testing.T) {
	page := orm.PageData[domain.UserSearchRow]{
		NextCursor: "300",
	}

	response := handler.ListUsersResponseFromPage(page)
	if response.GetNextCursor() != page.NextCursor {
		t.Errorf(
			"next cursor = %q, want %q",
			response.GetNextCursor(),
			page.NextCursor,
		)
	}
}
