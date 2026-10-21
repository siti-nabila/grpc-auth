package configs

import (
	"github.com/siti-nabila/grpc-auth/internal/features/application"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"time"
)

func prepareApplicationAccess(provider *database.TransactionProvider, metrics *telemetry.Store, now func() time.Time, eventID func() (string, error)) (*handler.ApplicationAccessHandler, error) {
	service, err := application.NewService(application.Dependencies{Transactions: provider, Snapshots: provider, Bind: func(tx domain.Transaction) application.Repositories {
		r := application.BindRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, metrics)
		return r
	}, Now: now, EventID: eventID})
	if err != nil {
		return nil, err
	}
	return handler.NewApplicationAccessHandler(service)
}

func prepareRegistration(provider *database.TransactionProvider, metrics *telemetry.Store, now func() time.Time, eventID func() (string, error)) (*handler.PendingRegistrationHandler, error) {
	registration, err := authfeature.NewPendingRegistrationService(authfeature.PendingRegistrationDependencies{
		Transactions: provider, Bind: func(tx domain.Transaction) authfeature.PendingRegistrationRepositories {
			r := authfeature.BindPendingRegistrationRepositories(tx)
			r.Audit = observeRequiredAudit(r.Audit, metrics)
			return r
		},
		HashPassword: authfeature.DefaultArgon2Config().HashPassword, Now: now, EventID: eventID,
	})
	if err != nil {
		return nil, err
	}
	register, err := handler.NewPendingRegistrationHandler(registration)
	if err != nil {
		return nil, err
	}
	return register, nil
}

func prepareProfile(provider *database.TransactionProvider, metrics *telemetry.Store, now func() time.Time, eventID func() (string, error)) (*handler.SessionProfileHandler, error) {
	profileService, err := profile.NewSessionProfileService(profile.SessionDependencies{
		Transactions: provider, Bind: func(tx domain.Transaction) profile.SessionRepositories {
			r := profile.BindSessionRepositories(tx)
			r.Audit = observeRequiredAudit(r.Audit, metrics)
			return r
		}, Now: now, EventID: eventID,
	})
	if err != nil {
		return nil, err
	}
	profileHandler, err := handler.NewSessionProfileHandler(profileService)
	if err != nil {
		return nil, err
	}
	return profileHandler, nil
}

func prepareUserManagement(provider *database.TransactionProvider, metrics *telemetry.Store, fingerprinter *common.Fingerprinter, now func() time.Time, eventID func() (string, error)) (*handler.UserManagementHandler, error) {
	management, err := user.NewManagementService(user.ManagementDependencies{Transactions: provider, Bind: func(tx domain.Transaction) user.ManagementRepositories {
		r := user.BindManagementRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, metrics)
		return r
	}, Fingerprinter: fingerprinter, HashPassword: authfeature.DefaultArgon2Config().HashPassword, Now: now, EventID: eventID})
	if err != nil {
		return nil, err
	}
	managementHandler, err := handler.NewUserManagementHandler(management)
	if err != nil {
		return nil, err
	}
	return managementHandler, nil
}

func prepareAssignments(provider *database.TransactionProvider, metrics *telemetry.Store, now func() time.Time, eventID func() (string, error)) (*handler.UserRoleAssignmentHandler, error) {
	assignments, err := user.NewAssignmentService(user.AssignmentDependencies{Transactions: provider, Snapshots: provider, Bind: func(tx domain.Transaction) user.AssignmentRepositories {
		r := user.BindAssignmentRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, metrics)
		return r
	}, Now: now, EventID: eventID})
	if err != nil {
		return nil, err
	}
	assignmentHandler, err := handler.NewUserRoleAssignmentHandler(assignments)
	if err != nil {
		return nil, err
	}
	return assignmentHandler, nil
}
