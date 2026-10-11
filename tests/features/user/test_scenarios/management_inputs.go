package test_scenarios

import (
	"strings"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func ManagementInputs() []testutils.Scenario {
	return []testutils.Scenario{{Name: "management profile fields preserve presence and reject privilege changes", Run: managementProfileInputs}, {Name: "creation target status key and schema bounds are explicit", Run: managementCreationInputs}, {Name: "complete user status transition matrix", Run: managementTransitions}}
}

func managementProfileInputs(t *testing.T) {
	value := "Ana"
	empty := ""
	valid := domain.UpdateUserInput{UserID: 42, ExpectedVersion: 1, Name: &value, Fields: []string{"name"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*domain.UpdateUserInput){
		"missing version":     func(r *domain.UpdateUserInput) { r.ExpectedVersion = 0 },
		"missing id":          func(r *domain.UpdateUserInput) { r.UserID = 0 },
		"overflow id":         func(r *domain.UpdateUserInput) { r.UserID = 2147483648 },
		"empty mask":          func(r *domain.UpdateUserInput) { r.Fields = nil },
		"duplicate mask":      func(r *domain.UpdateUserInput) { r.Fields = []string{"name", "name"} },
		"empty name":          func(r *domain.UpdateUserInput) { r.Name = &empty },
		"omitted selected":    func(r *domain.UpdateUserInput) { r.Name = nil },
		"unselected supplied": func(r *domain.UpdateUserInput) { r.Address = &empty },
		"oversized key":       func(r *domain.UpdateUserInput) { r.Key = strings.Repeat("k", 201) },
		"blank key":           func(r *domain.UpdateUserInput) { r.Key = " " },
	} {
		t.Run(name, func(t *testing.T) {
			r := valid
			change(&r)
			if r.Validate() == nil {
				t.Fatal("invalid update accepted")
			}
		})
	}
	for _, field := range []string{"email", "password", "status", "owner", "user_id", "roles", "application_access", "auth_version"} {
		r := valid
		r.Fields = []string{field}
		if r.Validate() == nil {
			t.Fatal("forbidden update field accepted", field)
		}
	}
	clear := domain.UpdateUserInput{UserID: 42, ExpectedVersion: 1, Address: &empty, Phone: &empty, Fields: []string{"address", "phone"}}
	if err := clear.Validate(); err != nil {
		t.Fatal("clear was rejected", err)
	}
	tooLong := strings.Repeat("x", 251)
	clear.Address = &tooLong
	if clear.Validate() == nil {
		t.Fatal("oversized address accepted")
	}
	tooLong = strings.Repeat("p", 21)
	clear.Address = &empty
	clear.Phone = &tooLong
	if clear.Validate() == nil {
		t.Fatal("oversized phone accepted")
	}
}

func managementCreationInputs(t *testing.T) {
	status := types.UserActive
	valid := domain.CreateUserInput{Name: "Ana", Email: " ANA@EXAMPLE.TEST ", Password: "synthetic-password", Status: &status, Key: "creation-1"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	if valid.Normalized().Email != "ana@example.test" {
		t.Fatal("normalization changed")
	}
	for name, change := range map[string]func(*domain.CreateUserInput){
		"omitted status":    func(r *domain.CreateUserInput) { r.Status = nil },
		"inactive creation": func(r *domain.CreateUserInput) { s := types.UserSuspended; r.Status = &s },
		"missing key":       func(r *domain.CreateUserInput) { r.Key = "" },
		"short password":    func(r *domain.CreateUserInput) { r.Password = "short" },
		"long password":     func(r *domain.CreateUserInput) { r.Password = strings.Repeat("p", 51) },
		"bad email":         func(r *domain.CreateUserInput) { r.Email = "invalid" },
		"display email":     func(r *domain.CreateUserInput) { r.Email = "Ana <ana@example.test>" },
		"long name":         func(r *domain.CreateUserInput) { r.Name = strings.Repeat("n", 101) },
		"nul phone":         func(r *domain.CreateUserInput) { r.Phone = "a\x00b" },
	} {
		t.Run(name, func(t *testing.T) {
			r := valid
			change(&r)
			if r.Validate() == nil {
				t.Fatal("invalid creation accepted")
			}
		})
	}
	r := domain.ChangeUserStatusInput{UserID: 42, ExpectedVersion: 1, Status: &status, Reason: "Policy review", Key: "status-1"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Status = nil
	if r.Validate() == nil {
		t.Fatal("missing status accepted")
	}
	r.Status = &status
	r.Reason = " "
	if r.Validate() == nil {
		t.Fatal("missing reason accepted")
	}
}

func managementTransitions(t *testing.T) {
	allowed := map[types.UserStatus]map[types.UserStatus]bool{
		types.UserPending:     {types.UserPending: true, types.UserActive: true, types.UserDeactivated: true},
		types.UserActive:      {types.UserActive: true, types.UserSuspended: true, types.UserDeactivated: true},
		types.UserSuspended:   {types.UserSuspended: true, types.UserActive: true, types.UserDeactivated: true},
		types.UserDeactivated: {types.UserDeactivated: true, types.UserActive: true},
	}
	statuses := []types.UserStatus{types.UserPending, types.UserActive, types.UserSuspended, types.UserDeactivated, types.UserStatus("UNKNOWN")}
	for _, before := range statuses {
		for _, after := range statuses {
			if got := domain.UserStatusTransitionAllowed(before, after); got != allowed[before][after] {
				t.Fatal("transition", before, after, got)
			}
		}
	}
}
