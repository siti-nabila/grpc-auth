package paginator

import (
	"testing"

	"github.com/siti-nabila/orm/orm"
)

func TestBuildOffsetPagination(t *testing.T) {
	result, err := Build(orm.QueryOptions{
		Page:  2,
		Limit: 25,
		Sort: []orm.SortField{
			{Field: "name"},
		},
	}, Config{
		Mode:         ModeOffset,
		DefaultLimit: 10,
		MaxLimit:     50,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if result.Page != 2 || result.Limit != 25 {
		t.Fatalf("unexpected metadata: %+v", result)
	}
	if result.Options.Page != 2 || result.Options.Limit != 25 {
		t.Fatalf("unexpected query options: %+v", result.Options)
	}
	if result.Options.InMemoryOffset != nil {
		t.Fatalf("offset mode should not set in-memory offset: %+v", result.Options.InMemoryOffset)
	}
	if len(result.Options.Sort) != 1 || result.Options.Sort[0].Field != "name" {
		t.Fatalf("offset mode should keep request sort: %+v", result.Options.Sort)
	}
}

func TestBuildCursorPaginationUsesBatchLimitAndEmptyCursor(t *testing.T) {
	result, err := Build(orm.QueryOptions{
		Page:  2,
		Limit: 2,
	}, Config{
		Mode:         ModeCursor,
		DefaultLimit: 10,
		MaxLimit:     1000,
		BatchLimit:   1000,
		Cursor: &CursorConfig{
			Field:        "auth_id",
			Value:        "",
			RequestField: "last_id",
			Parser:       PositiveInt64Cursor,
		},
		DefaultSort: []orm.SortField{
			{Field: "auth_id", Desc: true},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if result.Page != 2 || result.Limit != 2 {
		t.Fatalf("unexpected metadata: %+v", result)
	}
	if result.Options.Page != 2 || result.Options.Limit != 2 {
		t.Fatalf("unexpected query options: %+v", result.Options)
	}
	if result.Options.InMemoryOffset == nil ||
		result.Options.InMemoryOffset.Cursor.Field != "auth_id" ||
		result.Options.InMemoryOffset.Cursor.Value != "" ||
		result.Options.InMemoryOffset.MaxLimit != 1000 {
		t.Fatalf("unexpected cursor options: %+v", result.Options.InMemoryOffset)
	}
	if len(result.Options.Sort) != 1 ||
		result.Options.Sort[0].Field != "auth_id" ||
		!result.Options.Sort[0].Desc {
		t.Fatalf("unexpected default sort: %+v", result.Options.Sort)
	}
}

func TestBuildCursorPaginationParsesCursorValue(t *testing.T) {
	result, err := Build(orm.QueryOptions{
		Page:  1,
		Limit: 20,
	}, Config{
		Mode:       ModeCursor,
		MaxLimit:   1000,
		BatchLimit: 500,
		Cursor: &CursorConfig{
			Field:        "product_id",
			Value:        "42",
			RequestField: "last_id",
			Parser:       PositiveInt64Cursor,
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if result.Options.InMemoryOffset == nil ||
		result.Options.InMemoryOffset.Cursor.Field != "product_id" ||
		result.Options.InMemoryOffset.Cursor.Value != int64(42) ||
		result.Options.InMemoryOffset.MaxLimit != 500 {
		t.Fatalf("unexpected cursor options: %+v", result.Options.InMemoryOffset)
	}
}

func TestBuildCursorPaginationRejectsInvalidCursorValue(t *testing.T) {
	_, err := Build(orm.QueryOptions{}, Config{
		Mode: ModeCursor,
		Cursor: &CursorConfig{
			Field:        "auth_id",
			Value:        "not-an-id",
			RequestField: "last_id",
			Parser:       PositiveInt64Cursor,
		},
	})
	if err == nil {
		t.Fatalf("expected invalid cursor error")
	}
}

func TestBuildNormalizesLimits(t *testing.T) {
	result, err := Build(orm.QueryOptions{
		Page:  -1,
		Limit: 999,
	}, Config{
		Mode:         ModeOffset,
		DefaultLimit: 20,
		MaxLimit:     50,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if result.Page != 1 || result.Limit != 50 {
		t.Fatalf("unexpected normalized metadata: %+v", result)
	}
}
