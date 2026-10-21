package profile_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/profile/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestSessionProfile(t *testing.T) { testutils.Run(t, test_scenarios.Session()) }
