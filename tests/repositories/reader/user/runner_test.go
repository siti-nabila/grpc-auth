package user_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/repositories/reader/user/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestUserReader(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}
