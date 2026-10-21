package audit_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/domain/audit/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestAuditDomain(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
