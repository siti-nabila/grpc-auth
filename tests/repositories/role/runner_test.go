package role_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/role/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestRolePersistence(t *testing.T) { testutils.Run(t, test_scenarios.Persistence()) }

func TestCatalogSQL(t *testing.T) { testutils.Run(t, test_scenarios.CatalogSQL()) }
