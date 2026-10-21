package test_scenarios

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "optional application code normalization and validation", Run: applicationCodes},
		{Name: "target ID boundaries and immutable normalization", Run: targetIDs},
	}
}

func applicationCodes(t *testing.T) {
	for _, tc := range []struct {
		code  string
		want  string
		valid bool
	}{
		{"", "", true},
		{" \t\n", "", true},
		{" iam ", "iam", true},
		{"sales_v2", "sales_v2", true},
		{"unknown", "unknown", true},
		{strings.Repeat("a", 50), strings.Repeat("a", 50), true},
		{strings.Repeat("a", 51), strings.Repeat("a", 51), false},
		{"IAM", "IAM", false},
		{"iam.*", "iam.*", false},
		{"iam.user", "iam.user", false},
		{"1sales", "1sales", false},
		{"sales-v2", "sales-v2", false},
		{"sales team", "sales team", false},
		{"sales\x00", "sales\x00", false},
		{"\xff", "\xff", false},
	} {
		t.Run(tc.code, func(t *testing.T) {
			self := domain.MyEffectivePermissionsInput{ApplicationCode: tc.code}
			admin := domain.UserEffectivePermissionsInput{UserID: 7, ApplicationCode: tc.code}
			for _, err := range []error{self.Validate(), admin.Validate()} {
				if (err == nil) != tc.valid || !tc.valid && !errors.Is(err, common.ErrBadRequest) {
					t.Fatalf("valid=%v, error=%v", tc.valid, err)
				}
			}
			if self.Normalized().ApplicationCode != tc.want || admin.Normalized().ApplicationCode != tc.want {
				t.Fatal("unexpected normalized application code")
			}
			if self.ApplicationCode != tc.code || admin.ApplicationCode != tc.code || admin.Normalized().UserID != 7 {
				t.Fatal("input or target mutated")
			}
		})
	}
}

func targetIDs(t *testing.T) {
	for _, tc := range []struct {
		id    uint64
		valid bool
	}{
		{0, false}, {1, true}, {math.MaxInt32, true}, {math.MaxInt32 + 1, false}, {math.MaxUint64, false},
	} {
		in := domain.UserEffectivePermissionsInput{UserID: tc.id, ApplicationCode: " iam "}
		err := in.Validate()
		if (err == nil) != tc.valid || !tc.valid && !errors.Is(err, common.ErrBadRequest) {
			t.Fatalf("id=%d valid=%v error=%v", tc.id, tc.valid, err)
		}
		if in.Normalized().UserID != tc.id || in.ApplicationCode != " iam " {
			t.Fatal("normalization mutated target or original input")
		}
	}
}
