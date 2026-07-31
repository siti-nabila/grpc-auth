package interceptors_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/interceptors/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestInterceptors(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}
