package ratelimit_test

import (
	"github.com/siti-nabila/grpc-auth/tests/ratelimit/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestLocalLimiter(t *testing.T)           { testutils.Run(t, test_scenarios.All()) }
func TestRefreshAtomicAdmission(t *testing.T) { testutils.Run(t, test_scenarios.RefreshAtomic()) }
