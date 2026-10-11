package session_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/session/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestSelfRevocation(t *testing.T) { testutils.Run(t, test_scenarios.SelfRevocation()) }

func TestSessionLists(t *testing.T) { testutils.Run(t, test_scenarios.SessionLists()) }

func TestAdminRevocation(t *testing.T) { testutils.Run(t, test_scenarios.AdminRevocation()) }
