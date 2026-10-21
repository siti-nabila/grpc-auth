package authfeature_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestSessionAuthentication(t *testing.T) { testutils.Run(t, test_scenarios.Sessions()) }
func TestInitialCredentials(t *testing.T)    { testutils.Run(t, test_scenarios.Credentials()) }

func TestSessionLogin(t *testing.T) { testutils.Run(t, test_scenarios.SessionLogin()) }

func TestFailedLoginAudit(t *testing.T) { testutils.Run(t, test_scenarios.FailedLoginAudit()) }

func TestLoginPassword(t *testing.T)       { testutils.Run(t, test_scenarios.LoginPassword()) }
func TestPendingRegistration(t *testing.T) { testutils.Run(t, test_scenarios.PendingRegistration()) }

func TestRefresh(t *testing.T) { testutils.Run(t, test_scenarios.Refresh()) }

func TestLogout(t *testing.T) { testutils.Run(t, test_scenarios.Logout()) }

func TestChangePassword(t *testing.T) { testutils.Run(t, test_scenarios.ChangePassword()) }

func TestCurrentPermission(t *testing.T) { testutils.Run(t, test_scenarios.CurrentPermission()) }
