package application_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/domain/application/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestApplicationAccessDomain(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
