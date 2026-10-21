package catalog_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/catalog/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestCatalogReads(t *testing.T) { testutils.Run(t, test_scenarios.Reads()) }
