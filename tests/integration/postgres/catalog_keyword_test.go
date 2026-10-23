//go:build postgres

package postgres_test

import (
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestPostgresCatalogKeyword(t *testing.T) { testutils.Run(t, test_scenarios.CatalogKeyword()) }

func TestPostgresCatalogKeywordQueries(t *testing.T) {
	testutils.Run(t, test_scenarios.CatalogKeywordQueries())
}

func TestPostgresCatalogKeywordConcurrency(t *testing.T) {
	testutils.Run(t, test_scenarios.CatalogKeywordConcurrency())
}

func TestPostgresCatalogKeywordCommands(t *testing.T) {
	testutils.Run(t, test_scenarios.CatalogKeywordCommands())
}

func TestPostgresCatalogKeywordPerformance(t *testing.T) {
	testutils.Run(t, test_scenarios.CatalogKeywordPerformance())
}

func TestPostgresCatalogKeywordUsers(t *testing.T) {
	testutils.Run(t, test_scenarios.CatalogKeywordUsers())
}
