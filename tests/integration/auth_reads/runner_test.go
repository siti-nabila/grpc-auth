package auth_reads_test

import (
	"github.com/siti-nabila/grpc-auth/tests/integration/auth_reads/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestReadSnapshots(t *testing.T)  { testutils.Run(t, test_scenarios.All()) }
func TestServiceIngress(t *testing.T) { testutils.Run(t, test_scenarios.ServiceIngress()) }

func TestLoginProducer(t *testing.T) { testutils.Run(t, test_scenarios.Producer()) }

func TestRegisteredServer(t *testing.T) { testutils.Run(t, test_scenarios.Registered()) }

func TestFailureAudit(t *testing.T) { testutils.Run(t, test_scenarios.FailureAudit()) }

func TestRegisteredLifecycle(t *testing.T) { testutils.Run(t, test_scenarios.RegisteredLifecycle()) }
func TestUserDetail(t *testing.T)          { testutils.Run(t, test_scenarios.UserDetail()) }
func TestRegisteredAssignments(t *testing.T) {
	testutils.Run(t, test_scenarios.RegisteredAssignments())
}

func TestRegisteredApplicationAccess(t *testing.T) {
	testutils.Run(t, test_scenarios.RegisteredApplicationAccess())
}

func TestRegisteredEffectivePermissions(t *testing.T) {
	testutils.Run(t, test_scenarios.RegisteredEffectivePermissions())
}

func TestRegisteredAccessEvaluation(t *testing.T) {
	testutils.Run(t, test_scenarios.RegisteredAccessEvaluation())
}

func TestRegisteredAuditLogReads(t *testing.T) {
	testutils.Run(t, test_scenarios.RegisteredAuditLogReads())
}
