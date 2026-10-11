package handler_test

import (
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/handler/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func TestHandler(t *testing.T) {
	testutils.Run(t, test_scenarios.All())
}

func TestSessionLoginHandler(t *testing.T)    { testutils.Run(t, test_scenarios.SessionLogin()) }
func TestSessionCommandHandlers(t *testing.T) { testutils.Run(t, test_scenarios.SessionCommands()) }

func TestRefreshHandler(t *testing.T) { testutils.Run(t, test_scenarios.RefreshHandler()) }

func TestLogoutHandler(t *testing.T) { testutils.Run(t, test_scenarios.LogoutHandler()) }

func TestSelfSessionHandler(t *testing.T) { testutils.Run(t, test_scenarios.SelfSessionHandler()) }

func TestChangePasswordHandler(t *testing.T) {
	testutils.Run(t, test_scenarios.ChangePasswordHandler())
}

func TestListSessionsHandler(t *testing.T) { testutils.Run(t, test_scenarios.ListSessionsHandler()) }

func TestAdminSessionsHandler(t *testing.T) { testutils.Run(t, test_scenarios.AdminSessionsHandler()) }

func TestUserManagementHandlers(t *testing.T) {
	testutils.Run(t, test_scenarios.UserManagementHandlers())
}
