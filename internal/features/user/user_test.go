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

func TestSearchUsersReplaysPreviousBatchAndUsesAuthIDCursor(t *testing.T) {
	var captured []orm.QueryOptions
	reader := &fakeUserReader{
		t: t,
		fn: func(opts orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error) {
			captured = append(captured, opts)
			nextCursor := ""
			if len(captured) == 1 {
				nextCursor = "450"
			} else {
				nextCursor = "300"
			}
			return orm.PageData[domain.UserSearchRow]{
				Items:      []domain.UserSearchRow{},
				Total:      1000,
				Page:       opts.Page,
				Limit:      opts.Limit,
				TotalPages: 2,
				HasNext:    false,
				HasPrev:    opts.Page > 1,
				NextCursor: nextCursor,
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

	if len(captured) != 2 {
		t.Fatalf("expected replay and final reader calls, got %d", len(captured))
	}
	if captured[0].Page != 1 || captured[0].Limit != 500 {
		t.Fatalf("unexpected replay query pagination: %+v", captured[0])
	}
	if captured[0].InMemoryOffset == nil ||
		captured[0].InMemoryOffset.Cursor.Field != domain.UserListCursorField ||
		captured[0].InMemoryOffset.Cursor.Value != "" {
		t.Fatalf("unexpected replay cursor: %+v", captured[0].InMemoryOffset)
	}
	if captured[1].Page != 1 || captured[1].Limit != 500 {
		t.Fatalf("unexpected final query pagination: %+v", captured[1])
	}
	if captured[1].InMemoryOffset == nil ||
		captured[1].InMemoryOffset.Cursor.Field != domain.UserListCursorField ||
		captured[1].InMemoryOffset.Cursor.Value != "450" {
		t.Fatalf("unexpected final cursor: %+v", captured[1].InMemoryOffset)
	}
	if page.Page != 3 || page.Limit != 500 || !page.HasPrev {
		t.Fatalf("public response should keep global pagination, got %+v", page)
	}
}

func TestSearchUsersReturnsEmptyWhenReplayFindsNoNextCursor(t *testing.T) {
	reader := &fakeUserReader{
		t: t,
		fn: func(opts orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error) {
			return orm.PageData[domain.UserSearchRow]{
				Items: []domain.UserSearchRow{},
				Page:  opts.Page,
				Limit: opts.Limit,
			}, nil
		},
	}
	svc := &userService{
		ctx:        context.Background(),
		userReader: reader,
	}

	page, err := svc.SearchUsers(domain.UserListRequest{
		LastID: "10",
		Query: orm.QueryOptions{
			Page:  3,
			Limit: 500,
			Sort: []orm.SortField{
				{Field: domain.UserListCursorField},
			},
		},
	})
	if err != nil {
		t.Fatalf("SearchUsers returned error: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("expected only replay call, got %d calls", reader.calls)
	}
	if len(page.Items) != 0 || page.Page != 3 || page.Limit != 500 || page.HasNext || !page.HasPrev {
		t.Fatalf("unexpected empty page response: %+v", page)
	}
}
