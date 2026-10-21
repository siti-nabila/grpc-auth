package assignment_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/domain/assignment/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestAssignmentDomain(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
