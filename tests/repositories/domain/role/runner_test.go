package role_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/repositories/domain/role/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestRoleInputs(t *testing.T) { testutils.Run(t, test_scenarios.Inputs()) }

func TestRoleResults(t *testing.T) { testutils.Run(t, test_scenarios.Results()) }

func TestCatalogQuery(t *testing.T) { testutils.Run(t, test_scenarios.Catalogs()) }
