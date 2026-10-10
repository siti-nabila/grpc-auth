package authfeature_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestRegisterTransaction(t *testing.T)   { testutils.Run(t, test_scenarios.All()) }
func TestSessionAuthentication(t *testing.T) { testutils.Run(t, test_scenarios.Sessions()) }
func TestInitialCredentials(t *testing.T)    { testutils.Run(t, test_scenarios.Credentials()) }

func TestSessionLogin(t *testing.T) { testutils.Run(t, test_scenarios.SessionLogin()) }

func TestFailedLoginAudit(t *testing.T) { testutils.Run(t, test_scenarios.FailedLoginAudit()) }

func TestLoginPassword(t *testing.T)       { testutils.Run(t, test_scenarios.LoginPassword()) }
func TestPendingRegistration(t *testing.T) { testutils.Run(t, test_scenarios.PendingRegistration()) }
