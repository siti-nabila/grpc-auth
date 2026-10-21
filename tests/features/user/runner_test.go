package user_test

import (
	"github.com/siti-nabila/grpc-auth/tests/features/user/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestUserListQuery(t *testing.T)    { testutils.Run(t, test_scenarios.UserListQuery()) }
func TestManagementInputs(t *testing.T) { testutils.Run(t, test_scenarios.ManagementInputs()) }
func TestManagement(t *testing.T)       { testutils.Run(t, test_scenarios.Management()) }

func TestManagementReplay(t *testing.T) { testutils.Run(t, test_scenarios.ManagementReplay()) }

func TestAssignments(t *testing.T) { testutils.Run(t, test_scenarios.Assignments()) }

func TestEffectivePermissions(t *testing.T) { testutils.Run(t, test_scenarios.EffectivePermissions()) }
