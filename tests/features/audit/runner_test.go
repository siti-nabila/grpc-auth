package audit_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/audit/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestAuditFeature(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
