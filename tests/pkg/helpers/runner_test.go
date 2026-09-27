package helpers_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/pkg/helpers/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestDatabaseErrorMapping(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}
