package test_scenarios

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	ormconfig "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{
			Name: "builds parameterized created date and role filters",
			Run:  buildsParameterizedFilters,
		},
	}
}

func buildsParameterizedFilters(t *testing.T) {
	// Arrange
	createdFrom := time.Date(2026, time.July, 31, 17, 0, 0, 0, time.UTC)
	createdTo := time.Date(2026, time.August, 31, 17, 0, 0, 0, time.UTC)
	db := orm.NewSqlQueryAdapter(
		context.Background(),
		nil,
		dialect.NewPostgres(),
		ormconfig.Config{},
	)
	query := reader.BuildUserListQuery(db, domain.UserListFilter{
		CreatedFrom: &createdFrom,
		CreatedTo:   &createdTo,
		RoleCodes:   []uint64{1, 2},
	})

	// Act
	result, err := query.DryRun()

	// Assert
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}
	for _, fragment := range []string{
		"SELECT DISTINCT",
		"JOIN user_role ur ON ur.user_id = a.id",
		"JOIN role r ON r.id = ur.role_id",
		"a.created_at >= $1",
		"a.created_at < $2",
		"r.role_code IN ($3, $4)",
	} {
		if !strings.Contains(result.Query, fragment) {
			t.Errorf("query = %q, want fragment %q", result.Query, fragment)
		}
	}
	if len(result.Args) != 4 {
		t.Fatalf("args = %#v, want four parameterized values", result.Args)
	}
	if actual, ok := result.Args[0].(time.Time); !ok || !actual.Equal(createdFrom) {
		t.Errorf("created_from arg = %#v, want %v", result.Args[0], createdFrom)
	}
	if actual, ok := result.Args[1].(time.Time); !ok || !actual.Equal(createdTo) {
		t.Errorf("created_to arg = %#v, want %v", result.Args[1], createdTo)
	}
	if result.Args[2] != uint64(1) || result.Args[3] != uint64(2) {
		t.Errorf("role args = %#v, want [1 2]", result.Args[2:])
	}
}
