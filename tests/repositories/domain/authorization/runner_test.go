package authorization_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/repositories/domain/authorization/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestEvaluateAccessInputs(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}
