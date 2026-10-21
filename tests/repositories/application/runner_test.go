package application_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/application/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestApplicationAccessPersistence(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
