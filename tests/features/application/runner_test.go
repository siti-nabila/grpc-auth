package application_test

import (
	scenarios "github.com/siti-nabila/grpc-auth/tests/features/application/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestApplicationAccess(t *testing.T) { testutils.Run(t, scenarios.Access()) }
