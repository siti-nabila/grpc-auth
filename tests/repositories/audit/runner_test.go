package audit_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/audit/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestAuditReader(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
