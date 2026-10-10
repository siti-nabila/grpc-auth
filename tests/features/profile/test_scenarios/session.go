package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/profile/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Session() []testutils.Scenario {
	return []testutils.Scenario{{Name: "locked profile recheck and required writes share one handle", Run: sessionUpdate}, {Name: "profile field mask validates selected safe fields", Run: profileFields}}
}

func sessionUpdate(t *testing.T) {
	for _, name := range []string{"success", "no-op", "partial", "partial clear", "empty mask", "begin", "busy", "lock", "state", "revoked", "suspended", "restricted", "version changed", "owner", "read", "update", "version", "audit", "event-id", "outbox", "commit", "panic", "cancel"} {
		t.Run(name, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, base, profilev1.ProfileService_UpdateProfile_FullMethodName))
			tx := &shared.Transaction{Context: ctx}
			provider := &shared.TransactionProvider{Tx: tx}
			cause := errors.New("controlled profile dependency")
			if name == "begin" {
				provider.Error = cause
			}
			if name == "busy" {
				tx.Busy = true
			}
			if name == "lock" {
				tx.LockError = cause
			}
			if name == "commit" {
				tx.CommitError = cause
			}
			step := func(op string) error {
				tx.Record(op)
				if name == op {
					return cause
				}
				return nil
			}
			current := domain.Profile{Id: 7, UserId: 42, Name: "Old", Address: "address", Phone: "123"}
			var audits []domain.AuditRequest
			var events []domain.EventEnvelope
			deps := profile.SessionDependencies{Transactions: provider, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) {
				if err := step("event-id"); err != nil {
					return "", err
				}
				return fixtures.AccessTokenID, nil
			}, Bind: func(handle domain.Transaction) profile.SessionRepositories {
				if handle != tx {
					t.Fatal("transaction identity changed")
				}
				tx.Record("bind")
				return profile.SessionRepositories{
					State: shared.StateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
						if got != ctx || id != fixtures.SessionID {
							t.Fatal("state context or owner changed")
						}
						err := step("state")
						state := fixtures.SessionState()
						switch name {
						case "revoked":
							state.SessionStatus = types.SessionRevoked
						case "suspended":
							state.UserStatus = types.UserSuspended
						case "restricted":
							state.MustChangePassword = true
						case "version changed":
							state.AuthVersion++
						}
						return state, err
					}},
					Reader: mocks.Reader{Get: func(id uint64) (domain.Profile, error) {
						if id != 42 {
							t.Fatal("spoofed owner")
						}
						err := step("read")
						value := current
						if name == "owner" {
							value.UserId = 999
						}
						return value, err
					}},
					Writer: mocks.Writer{Write: func(req *domain.UpdateProfileRequest) error {
						expectedName := "New"
						if name == "partial clear" {
							expectedName = current.Name
						}
						if req.Id != 7 || req.Name != expectedName || name != "partial" && (req.Address != "" || req.Phone != "") || name == "partial" && (req.Address != current.Address || req.Phone != current.Phone) {
							t.Fatal("unsafe or wrong replacement", req)
						}
						err := step("update")
						if name == "panic" {
							panic("private data")
						}
						if name == "cancel" {
							cancel()
							return ctx.Err()
						}
						return err
					}},
					Version: shared.VersionWriter{Write: func(id uint64, version int64) error {
						if id != 42 || version != 5 {
							t.Fatal("version or target changed")
						}
						return step("version")
					}},
					Audit:  shared.AuditWriter{Write: func(req domain.AuditRequest) error { audits = append(audits, req); return step("audit") }},
					Outbox: shared.OutboxWriter{Write: func(req domain.EventEnvelope) error { events = append(events, req); return step("outbox") }},
				}
			}}
			service, err := profile.NewSessionProfileService(deps)
			if err != nil {
				t.Fatal(err)
			}
			req := domain.ProfileUpdate{Name: "New"}
			if name == "no-op" {
				req = domain.ProfileUpdate{Name: current.Name, Address: current.Address, Phone: current.Phone}
			}
			if name == "partial" {
				req.MaskPresent = true
				req.Mask = []string{"name"}
			}
			if name == "partial clear" {
				req = domain.ProfileUpdate{MaskPresent: true, Mask: []string{"address", "phone"}}
			}
			if name == "empty mask" {
				req = domain.ProfileUpdate{MaskPresent: true}
			}
			res, err := service.UpdateProfile(ctx, req)
			full := []string{"begin", "lock:auth:user:42", "lock:auth:session:" + fixtures.SessionID, "bind", "state", "read", "update", "version", "audit", "event-id", "outbox", "commit"}
			want := full
			stops := map[string]int{"begin": 1, "busy": 2, "lock": 2, "state": 5, "revoked": 5, "suspended": 5, "restricted": 5, "version changed": 5, "read": 6, "owner": 6, "update": 7, "panic": 7, "cancel": 7, "version": 8, "audit": 9, "event-id": 10, "outbox": 11, "commit": 12}
			if n, ok := stops[name]; ok {
				want = append([]string(nil), full[:n]...)
				if n > 1 {
					want = append(want, "rollback")
				}
			}
			if name == "no-op" || name == "empty mask" {
				want = append(append([]string(nil), full[:6]...), "audit", "commit")
			}
			if !reflect.DeepEqual(tx.Calls, want) {
				t.Fatalf("calls %v want %v", tx.Calls, want)
			}
			if name == "success" || name == "partial" || name == "partial clear" || name == "no-op" || name == "empty mask" {
				if err != nil || res == nil || res.Profile.UserId != 42 || res.Changed != (name != "no-op" && name != "empty mask") || res.Version != map[bool]int64{true: 5, false: 4}[res.Changed] {
					t.Fatal("committed result", res, err)
				}
				if len(audits) != 1 || len(events) != map[bool]int{true: 1, false: 0}[res.Changed] {
					t.Fatal("audit/event counts")
				}
				record, err := audits[0].Record()
				if err != nil || record.ActorUserID == nil || *record.ActorUserID != 42 || record.ActorService != nil {
					t.Fatal("audit actor", err)
				}
				if res.Changed {
					if audits[0].Action != types.AuditUserProfileUpdated || events[0].AggregateVersion != 5 || events[0].CorrelationID != "request-test" || events[0].Data.UserID != 42 {
						t.Fatal("event/audit version")
					}
					if _, err := events[0].Record(); err != nil {
						t.Fatal(err)
					}
				} else if audits[0].Action != types.AuditNoOp || !audits[0].After.NoOp {
					t.Fatal("no-op audit")
				}
			} else {
				if err == nil || res != nil {
					t.Fatal("partial success", res, err)
				}
				if name == "busy" && !errors.Is(err, auth.ErrResourceBusy) || name == "restricted" && !errors.Is(err, common.ErrForbidden) || name == "owner" && !errors.Is(err, common.ErrForbidden) || name == "cancel" && !errors.Is(err, context.Canceled) || name == "panic" && !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("wrong failure", err)
				}
				if name == "revoked" || name == "suspended" || name == "version changed" {
					if !errors.Is(err, auth.ErrInvalidSession) {
						t.Fatal("stale principal accepted", err)
					}
				}
				if name == "begin" || name == "lock" || name == "state" || name == "read" || name == "update" || name == "version" || name == "audit" || name == "event-id" || name == "outbox" || name == "commit" {
					if !errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable) {
						t.Fatal("dependency cause lost", err)
					}
				}
			}
		})
	}
}

func profileFields(t *testing.T) {
	for _, req := range []domain.ProfileUpdate{{Name: ""}, {Name: "ok"}, {Name: "Valid", MaskPresent: true, Mask: []string{"email"}}, {Name: "Valid", MaskPresent: true, Mask: []string{"user_id"}}, {Phone: string([]byte{255}), MaskPresent: true, Mask: []string{"phone"}}} {
		ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, context.Background(), profilev1.ProfileService_UpdateProfile_FullMethodName))
		tx := &shared.Transaction{Context: ctx}
		provider := &shared.TransactionProvider{Tx: tx}
		s, err := profile.NewSessionProfileService(profile.SessionDependencies{Transactions: provider, Bind: func(domain.Transaction) profile.SessionRepositories { panic("unexpected bind") }, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { panic("unexpected event") }})
		if err != nil {
			t.Fatal(err)
		}
		if res, err := s.UpdateProfile(ctx, req); err == nil || res != nil || provider.Calls != 0 {
			t.Fatal("invalid safe fields reached transaction")
		}
	}
}
