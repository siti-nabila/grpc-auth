package test_scenarios

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Inputs() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "creation bounds namespace and canonical final permission set", Run: createInputs},
		{Name: "metadata mask preserves omitted versus selected empty intent", Run: updateInputs},
		{Name: "status and permission replacement require explicit intent version and reason", Run: replacementInputs},
		{Name: "system IAM legacy roles protected and disabled custom apps manageable", Run: protectedRoles},
	}
}

func text(s string) *string { return &s }

func createInputs(t *testing.T) {
	base := domain.CreateRoleInput{ApplicationCode: " clm ", Code: " clm.reviewer ", Name: " 审核员 ", Key: "create-1", PermissionCodes: []string{"clm.contract.update", " clm.contract.read ", "clm.contract.read"}, Description: text(" Details ")}
	n := base.Normalized()
	if err := n.Validate(); err != nil || n.ApplicationCode != "clm" || n.Code != "clm.reviewer" || n.Name != "审核员" || *n.Description != "Details" || !slices.Equal(n.PermissionCodes, []string{"clm.contract.read", "clm.contract.update"}) {
		t.Fatal("creation normalization", err)
	}
	if base.PermissionCodes[0] != "clm.contract.update" || *base.Description != " Details " {
		t.Fatal("normalization mutated caller state")
	}
	for _, c := range []struct {
		name string
		edit func(*domain.CreateRoleInput)
		want error
	}{
		{"empty set allowed", func(r *domain.CreateRoleInput) { r.PermissionCodes = nil }, nil},
		{"rune bounds accepted", func(r *domain.CreateRoleInput) {
			r.Name = strings.Repeat("审", 50)
			r.Description = text(strings.Repeat("审", 100))
		}, nil},
		{"name too long", func(r *domain.CreateRoleInput) { r.Name = strings.Repeat("审", 51) }, common.ErrBadRequest},
		{"description too long", func(r *domain.CreateRoleInput) { r.Description = text(strings.Repeat("x", 101)) }, common.ErrBadRequest},
		{"empty name", func(r *domain.CreateRoleInput) { r.Name = "  " }, common.ErrBadRequest},
		{"invalid utf8", func(r *domain.CreateRoleInput) { r.Name = string([]byte{255}) }, common.ErrBadRequest},
		{"embedded null", func(r *domain.CreateRoleInput) { r.Description = text("a\x00b") }, common.ErrBadRequest},
		{"upper case not folded", func(r *domain.CreateRoleInput) { r.Code = "CLM.reviewer" }, common.ErrBadRequest},
		{"role wrong prefix", func(r *domain.CreateRoleInput) { r.Code = "iam.reviewer" }, common.ErrBadRequest},
		{"role third segment", func(r *domain.CreateRoleInput) { r.Code = "clm.contract.reviewer" }, common.ErrBadRequest},
		{"role code bound", func(r *domain.CreateRoleInput) { r.Code = "clm." + strings.Repeat("a", 97) }, common.ErrBadRequest},
		{"code100 accepted", func(r *domain.CreateRoleInput) { r.Code = "clm." + strings.Repeat("a", 96) }, nil},
		{"empty key", func(r *domain.CreateRoleInput) { r.Key = " " }, common.ErrBadRequest},
		{"key too long", func(r *domain.CreateRoleInput) { r.Key = strings.Repeat("k", 201) }, common.ErrBadRequest},
		{"IAM permission", func(r *domain.CreateRoleInput) { r.PermissionCodes = []string{"iam.role.update"} }, roleerrors.ErrInvalidPermissionSet},
		{"permission upper case", func(r *domain.CreateRoleInput) { r.PermissionCodes = []string{"clm.Contract.read"} }, roleerrors.ErrInvalidPermissionSet},
		{"permission resource subsegments", func(r *domain.CreateRoleInput) { r.PermissionCodes = []string{"clm.contract.status.update"} }, nil},
		{"permission150 accepted", func(r *domain.CreateRoleInput) {
			r.PermissionCodes = []string{"clm." + strings.Repeat("a", 141) + ".read"}
		}, nil},
		{"permission151 rejected", func(r *domain.CreateRoleInput) {
			r.PermissionCodes = []string{"clm." + strings.Repeat("a", 142) + ".read"}
		}, roleerrors.ErrInvalidPermissionSet},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := base
			c.edit(&r)
			if err := r.Validate(); !errors.Is(err, c.want) {
				t.Fatalf("got %v want %v", err, c.want)
			}
		})
	}
}

func updateInputs(t *testing.T) {
	base := domain.UpdateRoleInput{RoleID: 7, ExpectedVersion: 3, Description: text(""), Fields: []string{"description"}}
	for _, c := range []struct {
		name  string
		edit  func(*domain.UpdateRoleInput)
		valid bool
	}{
		{"selected empty clear", func(*domain.UpdateRoleInput) {}, true},
		{"missing version", func(r *domain.UpdateRoleInput) { r.ExpectedVersion = 0 }, false},
		{"int64 max version", func(r *domain.UpdateRoleInput) { r.ExpectedVersion = math.MaxInt64 }, true},
		{"zero target", func(r *domain.UpdateRoleInput) { r.RoleID = 0 }, false},
		{"target overflow", func(r *domain.UpdateRoleInput) { r.RoleID = math.MaxInt32 + 1 }, false},
		{"target max", func(r *domain.UpdateRoleInput) { r.RoleID = math.MaxInt32 }, true},
		{"missing mask", func(r *domain.UpdateRoleInput) { r.Fields = nil }, false},
		{"duplicate mask", func(r *domain.UpdateRoleInput) { r.Fields = []string{"description", "description"} }, false},
		{"forbidden code", func(r *domain.UpdateRoleInput) { r.Fields = []string{"description", "code"} }, false},
		{"mask without value", func(r *domain.UpdateRoleInput) { r.Description = nil }, false},
		{"value without mask", func(r *domain.UpdateRoleInput) { r.Name = text("Name") }, false},
		{"empty name invalid", func(r *domain.UpdateRoleInput) { r.Name = text(" "); r.Fields = []string{"name", "description"} }, false},
		{"optional key supplied", func(r *domain.UpdateRoleInput) { r.Key = "key" }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := base
			c.edit(&r)
			if (r.Validate() == nil) != c.valid {
				t.Fatal("invalid metadata validation outcome")
			}
		})
	}
	if r := base.Normalized(); r.Name != nil || r.Description == nil || *r.Description != "" {
		t.Fatal("presence intent lost")
	}
}

func replacementInputs(t *testing.T) {
	status := types.RoleDisabled
	change := domain.ChangeRoleStatusInput{RoleID: 7, ExpectedVersion: 3, Status: &status, Reason: " Policy review ", Key: "key"}
	if change.Validate() != nil || change.Normalized().Reason != "Policy review" {
		t.Fatal("valid status input rejected")
	}
	for _, bad := range []domain.ChangeRoleStatusInput{
		{RoleID: 7, ExpectedVersion: 3, Reason: "Policy", Key: "key"},
		{RoleID: 7, ExpectedVersion: 3, Status: &status, Reason: "no", Key: "key"},
		{RoleID: 7, ExpectedVersion: 3, Status: &status, Reason: "Policy"},
	} {
		if bad.Validate() == nil {
			t.Fatal("invalid status input accepted")
		}
	}
	set := domain.ReplaceRolePermissionsInput{RoleID: 7, ExpectedVersion: 3, Permissions: &domain.PermissionSet{}, Reason: "Clear final set", Key: "key"}
	if set.Validate() != nil {
		t.Fatal("present empty final set rejected")
	}
	set.Permissions = nil
	if set.Validate() == nil {
		t.Fatal("absent final-set intent accepted")
	}
	set.Permissions = &domain.PermissionSet{Codes: []string{"clm.contract.read", "clm.contract.read"}}
	if got := set.Normalized(); got.Validate() != nil || len(got.Permissions.Codes) != 1 || len(set.Permissions.Codes) != 2 {
		t.Fatal("dedup must preserve caller input")
	}
	set.ExpectedVersion = -1
	if set.Validate() == nil {
		t.Fatal("negative version accepted")
	}
}

func protectedRoles(t *testing.T) {
	for _, role := range []domain.RoleSummary{
		{Code: "clm.custom", ApplicationCode: "clm", IsSystem: true},
		{Code: "iam.legacy_8", ApplicationCode: types.ApplicationIAM},
		{Code: "iam.platform_admin", ApplicationCode: "clm"},
	} {
		if !errors.Is(role.Protect(), roleerrors.ErrSystemRoleProtected) {
			t.Fatal("system role not protected")
		}
	}
	if (domain.RoleSummary{Code: "clm.custom", ApplicationCode: "clm", Status: types.RoleDisabled}).Protect() != nil {
		t.Fatal("disabled custom role must remain manageable")
	}
	if !errors.Is((domain.ApplicationItem{Code: types.ApplicationIAM, Status: types.ApplicationActive}).AllowCustomRole(), applicationerrors.ErrSystemApplicationProtected) {
		t.Fatal("IAM custom creation allowed")
	}
	if !errors.Is((domain.ApplicationItem{Code: "clm", Status: types.ApplicationDisabled}).AllowCustomRole(), applicationerrors.ErrApplicationInactive) {
		t.Fatal("inactive app creation allowed")
	}
	if (domain.ApplicationItem{Code: "clm", Status: types.ApplicationActive}).AllowCustomRole() != nil {
		t.Fatal("active business app denied")
	}
}
