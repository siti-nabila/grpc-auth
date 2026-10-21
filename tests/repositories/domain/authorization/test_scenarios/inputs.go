package test_scenarios

import (
	"errors"
	"strings"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func All() []testutils.Scenario {
	valid := []domain.EvaluateAccessInput{
		{ApplicationCode: "iam", PermissionCode: "iam.user.read"},
		{ApplicationCode: " iam ", PermissionCode: " iam.user.status.update "},
		{ApplicationCode: "unknown", PermissionCode: "unknown.resource.read"},
		{ApplicationCode: strings.Repeat("a", 50), PermissionCode: strings.Repeat("a", 50) + ".resource.read"},
		{ApplicationCode: "iam", PermissionCode: "iam." + strings.Repeat("r", 141) + ".read"},
	}
	invalid := []domain.EvaluateAccessInput{
		{},
		{ApplicationCode: "iam"},
		{PermissionCode: "iam.user.read"},
		{ApplicationCode: " ", PermissionCode: "iam.user.read"},
		{ApplicationCode: "iam", PermissionCode: " "},
		{ApplicationCode: "IAM", PermissionCode: "IAM.user.read"},
		{ApplicationCode: "iam", PermissionCode: "iam.User.read"},
		{ApplicationCode: "iam", PermissionCode: "iam.*.read"},
		{ApplicationCode: "iam", PermissionCode: "iam..read"},
		{ApplicationCode: "iam", PermissionCode: "iam.read"},
		{ApplicationCode: "iam", PermissionCode: "iam.user.read."},
		{ApplicationCode: "iam", PermissionCode: "iam.user-read.read"},
		{ApplicationCode: "clm", PermissionCode: "iam.user.read"},
		{ApplicationCode: "iam", PermissionCode: "iams.user.read"},
		{ApplicationCode: "iam.user", PermissionCode: "iam.user.read"},
		{ApplicationCode: "1iam", PermissionCode: "1iam.user.read"},
		{ApplicationCode: strings.Repeat("a", 51), PermissionCode: strings.Repeat("a", 51) + ".resource.read"},
		{ApplicationCode: "iam", PermissionCode: "iam." + strings.Repeat("r", 142) + ".read"},
		{ApplicationCode: "iam", PermissionCode: "iam.user.\x00read"},
	}
	out := []testutils.Scenario{}
	for i, input := range valid {
		out = append(out, testutils.Scenario{Name: "valid exact permission " + string(rune('A'+i)), Run: func(t *testing.T) {
			before := input
			if err := input.Validate(); err != nil {
				t.Fatal(err)
			}
			normalized := input.Normalized()
			if input != before || normalized.ApplicationCode != strings.TrimSpace(input.ApplicationCode) || normalized.PermissionCode != strings.TrimSpace(input.PermissionCode) || normalized.Normalized() != normalized {
				t.Fatal("normalization changed input or is not stable", input, normalized)
			}
		}})
	}
	for i, input := range invalid {
		out = append(out, testutils.Scenario{Name: "invalid exact permission " + string(rune('A'+i)), Run: func(t *testing.T) {
			if err := input.Validate(); !errors.Is(err, common.ErrBadRequest) {
				t.Fatal("expected shared validation error", input, err)
			}
		}})
	}
	return out
}
