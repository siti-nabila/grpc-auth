package transaction_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/transaction/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestBoundRepositories(t *testing.T)  { testutils.Run(t, test_scenarios.All()) }
func TestPersistence(t *testing.T)        { testutils.Run(t, test_scenarios.Persistence()) }
func TestIdempotencyStore(t *testing.T)   { testutils.Run(t, test_scenarios.IdempotencyStore()) }
func TestSessionPersistence(t *testing.T) { testutils.Run(t, test_scenarios.Session()) }
func TestPermissionSnapshot(t *testing.T) { testutils.Run(t, test_scenarios.Authorization()) }
func TestLoginPersistence(t *testing.T)   { testutils.Run(t, test_scenarios.LoginPersistence()) }
