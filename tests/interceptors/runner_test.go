package interceptors_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/interceptors/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestInterceptors(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}

func TestAuthRateLimiter(t *testing.T)  { testutils.Run(t, test_scenarios.Rate()) }
func TestSecurityAttempts(t *testing.T) { testutils.Run(t, test_scenarios.SecurityAttempts()) }

func TestLifecycleInterceptors(t *testing.T) { testutils.Run(t, test_scenarios.Lifecycle()) }
func TestLifecycleRateGroups(t *testing.T)   { testutils.Run(t, test_scenarios.LifecycleRateGroups()) }

func TestRoleRateGroups(t *testing.T) { testutils.Run(t, test_scenarios.RoleRateGroups()) }
