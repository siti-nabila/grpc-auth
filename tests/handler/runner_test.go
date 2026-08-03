package handler_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/handler/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestHandler(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}
