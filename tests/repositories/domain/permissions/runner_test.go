package permissions_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/repositories/domain/permissions/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestEffectivePermissionsInputs(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}
