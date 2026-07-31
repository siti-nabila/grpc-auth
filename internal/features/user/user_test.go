package user

import (
	"context"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
)

type fakeUserReader struct {
	t     *testing.T
	calls int
	fn    func(orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error)
}

func (f *fakeUserReader) SearchUsers(opts orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error) {
	f.calls++
	if f.fn == nil {
		f.t.Fatalf("unexpected SearchUsers call")
	}
	return f.fn(opts)
}

func TestSearchUsersUsesLastIDAsMovingAuthIDCursor(t *testing.T) {
	var captured []orm.QueryOptions
	reader := &fakeUserReader{
		t: t,
		fn: func(opts orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error) {
			captured = append(captured, opts)
			return orm.PageData[domain.UserSearchRow]{
				Items:      []domain.UserSearchRow{},
				Total:      1000,
				Page:       opts.Page,
				Limit:      opts.Limit,
				TotalPages: 2,
				HasNext:    false,
				HasPrev:    opts.Page > 1,
				NextCursor: "300",
			}, nil
		},
	}
	svc := &userService{
		ctx:        context.Background(),
		userReader: reader,
	}

	page, err := svc.SearchUsers(domain.UserListRequest{
		LastID: "400",
		Query: orm.QueryOptions{
			Page:  3,
			Limit: 500,
			Sort: []orm.SortField{
				{Field: domain.UserListCursorField, Desc: true},
			},
			Search: &orm.SearchQuery{
				Fields:  []string{"keyword"},
				Keyword: "stored-cursor",
				Mode:    orm.SearchModeFullTextTrigram,
			},
		},
	})
	if err != nil {
		t.Fatalf("SearchUsers returned error: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected one cursor reader call, got %d", len(captured))
	}
	if captured[0].Page != 3 || captured[0].Limit != 500 {
		t.Fatalf("unexpected cursor query pagination: %+v", captured[0])
	}
	if captured[0].InMemoryOffset == nil ||
		captured[0].InMemoryOffset.Cursor.Field != domain.UserListCursorField ||
		captured[0].InMemoryOffset.Cursor.Value != int64(400) ||
		captured[0].InMemoryOffset.MaxLimit != userListMaxLimit {
		t.Fatalf("unexpected cursor options: %+v", captured[0].InMemoryOffset)
	}
	if page.Page != 3 || page.Limit != 500 || !page.HasPrev || page.NextCursor != "300" {
		t.Fatalf("public response should keep global pagination, got %+v", page)
	}
}

func TestSearchUsersUsesEmptyLastIDForFirstBatchInMemoryPagination(t *testing.T) {
	var captured []orm.QueryOptions
	reader := &fakeUserReader{
		t: t,
		fn: func(opts orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error) {
			captured = append(captured, opts)
			return orm.PageData[domain.UserSearchRow]{
				Items: []domain.UserSearchRow{
					{AuthID: 998},
					{AuthID: 997},
				},
				Total:      1000,
				Page:       opts.Page,
				Limit:      opts.Limit,
				TotalPages: 500,
				HasNext:    true,
				HasPrev:    opts.Page > 1,
				NextCursor: "997",
			}, nil
		},
	}
	svc := &userService{
		ctx:        context.Background(),
		userReader: reader,
	}

	page, err := svc.SearchUsers(domain.UserListRequest{
		Query: orm.QueryOptions{
			Page:  2,
			Limit: 2,
			Sort: []orm.SortField{
				{Field: domain.UserListCursorField, Desc: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("SearchUsers returned error: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("expected one reader call without cursor, got %d calls", len(captured))
	}
	if captured[0].Page != 2 || captured[0].Limit != 2 {
		t.Fatalf("unexpected first batch pagination: %+v", captured[0])
	}
	if captured[0].InMemoryOffset == nil ||
		captured[0].InMemoryOffset.Cursor.Field != domain.UserListCursorField ||
		captured[0].InMemoryOffset.Cursor.Value != "" ||
		captured[0].InMemoryOffset.MaxLimit != userListMaxLimit {
		t.Fatalf("unexpected first batch cursor options: %+v", captured[0].InMemoryOffset)
	}
	if len(page.Items) != 2 || page.Page != 2 || page.Limit != 2 || !page.HasNext || !page.HasPrev || page.NextCursor != "997" {
		t.Fatalf("unexpected first batch page response: %+v", page)
	}
}

func TestSearchUsersRejectsInvalidLastID(t *testing.T) {
	reader := &fakeUserReader{
		t: t,
	}
	svc := &userService{
		ctx:        context.Background(),
		userReader: reader,
	}

	_, err := svc.SearchUsers(domain.UserListRequest{
		LastID: "not-an-id",
		Query: orm.QueryOptions{
			Page:  1,
			Limit: 500,
		},
	})
	if err == nil {
		t.Fatalf("expected invalid last_id error")
	}
	if reader.calls != 0 {
		t.Fatalf("expected no reader call with invalid last_id, got %d calls", reader.calls)
	}
}
