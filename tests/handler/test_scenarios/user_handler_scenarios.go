package test_scenarios

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{
			Name: "maps next cursor into list users response",
			Run:  mapNextCursor,
		},
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
