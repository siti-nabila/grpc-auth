package authorization_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/features/authorization/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestEvaluateAccess(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
