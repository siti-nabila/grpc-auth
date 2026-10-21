package test_scenarios

import (
	"database/sql"
	"reflect"
	"testing"

	feature "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Authorization() []testutils.Scenario {
	return []testutils.Scenario{{Name: "permission union preserves contributing roles and authoritative gates", Run: permissionUnion}}
}

func permissionUnion(t *testing.T) {
	base := domain.PermissionContribution{PermissionCode: "iam.user.read", PermissionApplicationID: 1, RoleID: 2, RoleCode: "iam.platform_admin", RoleApplicationID: 1, RoleStatus: "ACTIVE", AssignmentStatus: "ACTIVE", ApplicationCode: "iam", ApplicationStatus: "ACTIVE"}
	other := base
	other.RoleID = 3
	other.RoleCode = "iam.auditor"
	want := []domain.EffectivePermission{{Code: "iam.user.read", Roles: []domain.PermissionRole{{ID: 2, Code: "iam.platform_admin"}, {ID: 3, Code: "iam.auditor"}}}}
	got := feature.ResolvePermissions([]domain.PermissionContribution{other, base, base})
	if !reflect.DeepEqual(got, want) || !feature.HasPermission(got, "iam.user.read") || feature.HasPermission(got, "iam.user.write") {
		t.Fatal("union or roles not preserved", got)
	}
	base.AssignmentStatus = "REVOKED"
	got = feature.ResolvePermissions([]domain.PermissionContribution{base, other})
	if len(got) != 1 || len(got[0].Roles) != 1 || got[0].Roles[0].ID != 3 {
		t.Fatal("partial revoke removed surviving contribution")
	}
	changes := []struct {
		name   string
		change func(*domain.PermissionContribution)
	}{
		{"revoked assignment", func(r *domain.PermissionContribution) { r.AssignmentStatus = "REVOKED" }},
		{"disabled role", func(r *domain.PermissionContribution) { r.RoleStatus = "DISABLED" }},
		{"disabled application", func(r *domain.PermissionContribution) { r.ApplicationStatus = "DISABLED" }},
		{"cross-application mapping", func(r *domain.PermissionContribution) { r.PermissionApplicationID = 2 }},
		{"wrong namespace", func(r *domain.PermissionContribution) { r.PermissionCode = "clm.contract.read" }},
		{"wildcard", func(r *domain.PermissionContribution) { r.PermissionCode = "iam.*" }},
		{"business without access", func(r *domain.PermissionContribution) {
			r.ApplicationCode = "clm"
			r.PermissionCode = "clm.contract.read"
		}},
		{"business revoked access", func(r *domain.PermissionContribution) {
			r.ApplicationCode = "clm"
			r.PermissionCode = "clm.contract.read"
			r.AccessStatus = sql.NullString{String: "REVOKED", Valid: true}
		}},
	}
	base = other
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.change(&r)
			if len(feature.ResolvePermissions([]domain.PermissionContribution{r})) != 0 {
				t.Fatal("inactive or foreign contribution granted")
			}
		})
	}
	business := base
	business.ApplicationCode = "clm"
	business.PermissionCode = "clm.contract.read"
	business.AccessStatus = sql.NullString{String: "ACTIVE", Valid: true}
	if got := feature.ResolvePermissions([]domain.PermissionContribution{business}); !feature.HasPermission(got, "clm.contract.read") {
		t.Fatal("active business access denied")
	}
	if len(feature.ResolvePermissions(nil)) != 0 {
		t.Fatal("implicit admin grant")
	}
}
