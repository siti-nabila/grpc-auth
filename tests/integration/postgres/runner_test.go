//go:build postgres

package postgres_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestPostgresConfiguration(t *testing.T) {
	testutils.Run(t, test_scenarios.Configuration())
}

func TestPostgresEnvironment(t *testing.T) {
	testutils.Run(t, test_scenarios.Environment())
}

func TestPostgresFoundationHarness(t *testing.T) {
	testutils.Run(t, test_scenarios.FoundationHarness())
}

func TestPostgresTransactionOwnership(t *testing.T) {
	testutils.Run(t, test_scenarios.TransactionOwnership())
}

func TestPostgresExistingCommands(t *testing.T) {
	testutils.Run(t, test_scenarios.ExistingCommands())
}
func TestPostgresPersistenceCommands(t *testing.T) {
	testutils.Run(t, test_scenarios.PersistenceCommands())
}

func TestPostgresLoginProducer(t *testing.T) {
	testutils.Run(t, test_scenarios.LoginProducer())
}
func TestPostgresSessionFoundation(t *testing.T) {
	testutils.Run(t, test_scenarios.SessionFoundation())
}
func TestPostgresSessionProfileConcurrency(t *testing.T) {
	testutils.Run(t, test_scenarios.SessionProfileConcurrency())
}
func TestPostgresSecurityAttempts(t *testing.T) {
	testutils.Run(t, test_scenarios.SecurityAttemptPersistence())
}

func TestPostgresRegisteredServer(t *testing.T) { testutils.Run(t, test_scenarios.RegisteredServer()) }

func TestPostgresRefreshLifecycle(t *testing.T) { testutils.Run(t, test_scenarios.RefreshLifecycle()) }

func TestPostgresSessionGeneration(t *testing.T) {
	testutils.Run(t, test_scenarios.SessionGeneration())
}

func TestPostgresLogoutLifecycle(t *testing.T) { testutils.Run(t, test_scenarios.LogoutLifecycle()) }

func TestPostgresSelfSessionLifecycle(t *testing.T) {
	testutils.Run(t, test_scenarios.SelfSessionLifecycle())
}

func TestPostgresPasswordLifecycle(t *testing.T) {
	testutils.Run(t, test_scenarios.PasswordLifecycle())
}

func TestPostgresSessionListLifecycle(t *testing.T) {
	testutils.Run(t, test_scenarios.SessionListLifecycle())
}

func TestPostgresAdminSessionLifecycle(t *testing.T) {
	testutils.Run(t, test_scenarios.AdminSessionLifecycle())
}

func TestPostgresUserListLifecycle(t *testing.T) {
	testutils.Run(t, test_scenarios.UserListLifecycle())
}

func TestPostgresRegisteredLifecycle(t *testing.T) {
	testutils.Run(t, test_scenarios.RegisteredLifecycle())
}

func TestPostgresUserManagement(t *testing.T) {
	testutils.Run(t, test_scenarios.UserManagement())
}

func TestPostgresUserManagementConcurrency(t *testing.T) {
	testutils.Run(t, test_scenarios.UserManagementConcurrency())
}
