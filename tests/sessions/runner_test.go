package sessions_test

import (
	"github.com/siti-nabila/grpc-auth/tests/sessions/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestLogoutCredential(t *testing.T) { testutils.Run(t, test_scenarios.LogoutCredential()) }
