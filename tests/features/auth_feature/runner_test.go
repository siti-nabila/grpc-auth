package authfeature_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestRegisterTransaction(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
