package common_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/common/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestTransactionOwnership(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
func TestIdempotency(t *testing.T)          { testutils.Run(t, test_scenarios.Idempotency()) }
