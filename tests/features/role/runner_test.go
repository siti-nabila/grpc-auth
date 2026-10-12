package role_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/role/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestRoleReads(t *testing.T) { testutils.Run(t, test_scenarios.Reads()) }

func TestRoleManagement(t *testing.T) { testutils.Run(t, test_scenarios.Management()) }

func TestRoleConstruction(t *testing.T) { testutils.Run(t, test_scenarios.Construction()) }
