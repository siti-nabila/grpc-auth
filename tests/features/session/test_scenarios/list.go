package test_scenarios

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	authmocks "github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/features/session/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"reflect"
	"strings"
	"testing"
	"time"
)

func SessionLists() []testutils.Scenario {
	return []testutils.Scenario{{Name: "session lists bind authority and safe page to one snapshot", Run: listMatrix}, {Name: "session composite cursors preserve each primary direction null and owner query binding", Run: listCursorMatrix}}
}

func listGrant() domain.PermissionContribution {
	return domain.PermissionContribution{PermissionCode: string(types.PermissionIAMSessionRead), PermissionApplicationID: 1, RoleID: 2, RoleCode: "iam.reader", RoleApplicationID: 1, RoleStatus: types.RoleActive, AssignmentStatus: types.AssignmentActive, ApplicationCode: types.ApplicationIAM, ApplicationStatus: types.ApplicationActive}
}

func listMatrix(t *testing.T) {
	for _, mode := range []string{"self", "admin"} {
		method := sessionv1.SessionService_ListMySessions_FullMethodName
		if mode == "admin" {
			method = sessionv1.SessionService_ListUserSessions_FullMethodName
		}
		path := []string{"snapshot", "bind", "state", "page", "commit"}
		if mode == "admin" {
			path = []string{"snapshot", "bind", "state", "permissions", "page", "commit"}
		}
		cases := []string{"success", "empty", "restricted", "locked restricted", "stale session", "missing caller", "missing principal", "incomplete", "foreign row", "invalid sort", "invalid owner", "invalid cursor", "negative page", "negative limit", "limit cap", "error:snapshot", "error:commit"}
		if mode == "admin" {
			cases = append(cases, "missing permission", "disabled role", "inactive assignment", "wrong namespace", "wildcard permission")
		}
		for _, stage := range path {
			if stage != "snapshot" && stage != "commit" && stage != "bind" {
				for _, failure := range []string{"error", "cancel", "panic"} {
					cases = append(cases, failure+":"+stage)
				}
			}
		}
		for _, kind := range cases {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx := fixtures.CallerContext(t, context.Background(), method, method)
				if kind == "missing caller" {
					ctx = context.Background()
				}
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				state := fixtures.SessionState()
				if kind == "restricted" {
					state.MustChangePassword = true
				}
				if kind != "missing principal" {
					p, e := sessions.PrincipalForState(fixtures.AccessClaims(t), state, fixtures.ServiceTime)
					if e != nil {
						t.Fatal(e)
					}
					ctx = p.Context(ctx)
				}
				tx := &shared.Transaction{Context: ctx}
				cause := errors.New("controlled private dependency")
				step := func(stage string) error {
					tx.Record(stage)
					switch kind {
					case "error:" + stage:
						return cause
					case "cancel:" + stage:
						cancel()
						return context.Canceled
					case "panic:" + stage:
						panic("private panic")
					}
					return nil
				}
				if kind == "error:commit" {
					tx.CommitError = cause
				}
				q := session.ListInput{}
				target := uint64(42)
				if mode == "admin" {
					target = 43
				}
				switch kind {
				case "invalid sort":
					q.Field = "password"
				case "invalid owner":
					if mode == "admin" {
						target = 0
					}
				case "invalid cursor":
					q.Cursor = "invalid"
				case "negative page":
					q.Page = -1
				case "negative limit":
					q.Limit = -1
				case "limit cap":
					q.Limit = 101
				}
				d := session.ListDependencies{Snapshots: mocks.Snapshot{Begin: func(got context.Context) (domain.Transaction, error) {
					if got != ctx {
						t.Fatal("snapshot context")
					}
					e := step("snapshot")
					return tx, e
				}}, Now: func() time.Time { return fixtures.ServiceTime }}
				d.Bind = func(handle domain.Transaction) session.ListRepositories {
					if handle != tx {
						t.Fatal("snapshot binding")
					}
					tx.Record("bind")
					if kind == "incomplete" {
						return session.ListRepositories{}
					}
					return session.ListRepositories{State: authmocks.SessionStateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
						if got != ctx || id != fixtures.SessionID {
							t.Fatal("authority scope")
						}
						fresh := state
						if kind == "stale session" {
							fresh.AuthVersion++
						}
						if kind == "locked restricted" {
							fresh.MustChangePassword = true
						}
						return fresh, step("state")
					}}, Permissions: mocks.Permissions{Read: func(got context.Context, id uint64) ([]domain.PermissionContribution, error) {
						if got != ctx || id != 42 {
							t.Fatal("permission scope")
						}
						g := listGrant()
						switch kind {
						case "missing permission":
							return nil, step("permissions")
						case "disabled role":
							g.RoleStatus = types.RoleDisabled
						case "inactive assignment":
							g.AssignmentStatus = types.AssignmentRevoked
						case "wrong namespace":
							g.PermissionCode = "clm.session.read"
						case "wildcard permission":
							g.PermissionCode = "iam.*"
						}
						return []domain.PermissionContribution{g}, step("permissions")
					}}, Sessions: mocks.Targets{Page: func(got context.Context, r domain.SessionListQuery) (domain.SessionListPage, error) {
						limit := 20
						if kind == "limit cap" {
							limit = 100
						}
						if got != ctx || r.UserID != target || r.Field != domain.SessionSortCreatedAt || !r.Desc || r.Limit != limit || r.After != nil {
							t.Fatal("normalized query")
						}
						page := domain.SessionListPage{Total: 1, Items: []domain.SessionSummary{{ID: fixtures.SessionID, UserID: target, Status: types.SessionActive, CreatedAt: state.CreatedAt, ExpiresAt: state.ExpiresAt}}}
						if kind == "empty" {
							page.Total = 0
							page.Items = nil
						}
						if kind == "foreign row" {
							page.Items[0].UserID = 99
						}
						return page, step("page")
					}}}
				}
				svc, e := session.NewListService(d)
				if e != nil {
					t.Fatal(e)
				}
				var res *session.ListResult
				var err error
				if mode == "self" {
					res, err = svc.ListMySessions(ctx, q)
				} else {
					res, err = svc.ListUserSessions(ctx, target, q)
				}
				stop := map[string]string{"locked restricted": "state", "stale session": "state", "incomplete": "bind", "foreign row": "page", "missing permission": "permissions", "disabled role": "permissions", "inactive assignment": "permissions", "wrong namespace": "permissions", "wildcard permission": "permissions", "error:snapshot": "snapshot", "error:commit": "commit"}[kind]
				for _, stage := range path {
					if kind == "error:"+stage || kind == "cancel:"+stage || kind == "panic:"+stage {
						stop = stage
					}
				}
				noCalls := kind == "missing caller" || kind == "missing principal" || kind == "restricted" || kind == "invalid sort" || kind == "invalid cursor" || kind == "negative page" || kind == "negative limit" || kind == "invalid owner" && mode == "admin"
				want := append([]string{}, path...)
				if noCalls {
					want = nil
				} else if stop != "" {
					for n, stage := range want {
						if stage == stop {
							want = want[:n+1]
							if stage != "snapshot" {
								want = append(want, "rollback")
							}
							break
						}
					}
				}
				if !reflect.DeepEqual(tx.Calls, want) {
					t.Fatalf("calls %v want %v", tx.Calls, want)
				}
				if noCalls || stop != "" {
					if res != nil || err == nil {
						t.Fatal("failure outcome", err)
					}
					if strings.HasPrefix(kind, "error:") && !errors.Is(err, cause) {
						t.Fatal("cause lost", err)
					}
					if strings.HasPrefix(kind, "cancel:") && !errors.Is(err, context.Canceled) {
						t.Fatal("cancel", err)
					}
					if strings.HasPrefix(kind, "panic:") && !errors.Is(err, common.ErrInternalServerError) {
						t.Fatal("panic", err)
					}
					return
				}
				if err != nil || res == nil || res.Page != 1 || res.HasPrev || res.HasNext {
					t.Fatal("list result", err)
				}
				if kind == "empty" {
					if res.Total != 0 || len(res.Items) != 0 || res.NextCursor != "" {
						t.Fatal("empty metadata")
					}
				} else if len(res.Items) != 1 || res.Total != 1 || res.NextCursor == "" || !res.Items[0].Current {
					t.Fatal("safe item/current metadata")
				}
			})
		}
	}
	if _, e := session.NewListService(session.ListDependencies{}); e == nil {
		t.Fatal("incomplete dependencies")
	}
}

func listCursorMatrix(t *testing.T) {
	for _, field := range []domain.SessionSortField{domain.SessionSortCreatedAt, domain.SessionSortLastSeenAt, domain.SessionSortExpiresAt} {
		for _, desc := range []bool{false, true} {
			for _, null := range []bool{false, true} {
				if null && field != domain.SessionSortLastSeenAt {
					continue
				}
				t.Run(string(field)+map[bool]string{true: " DESC", false: " ASC"}[desc]+map[bool]string{true: " null", false: " value"}[null], func(t *testing.T) {
					method := sessionv1.SessionService_ListUserSessions_FullMethodName
					ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, context.Background(), method, method))
					row := domain.SessionSummary{ID: fixtures.SessionID, UserID: 42, Status: types.SessionActive, CreatedAt: fixtures.ServiceTime.Add(-time.Hour), ExpiresAt: fixtures.ServiceTime.Add(time.Hour), LastSeenAt: sql.NullTime{Time: fixtures.ServiceTime, Valid: !null}}
					var gotQuery domain.SessionListQuery
					tx := &shared.Transaction{Context: ctx}
					svc, e := session.NewListService(session.ListDependencies{Snapshots: mocks.Snapshot{Begin: func(context.Context) (domain.Transaction, error) { return tx, nil }}, Now: func() time.Time { return fixtures.ServiceTime }, Bind: func(domain.Transaction) session.ListRepositories {
						return session.ListRepositories{State: authmocks.SessionStateReader{Read: func(context.Context, string) (domain.SessionState, error) { return fixtures.SessionState(), nil }}, Permissions: mocks.Permissions{Read: func(context.Context, uint64) ([]domain.PermissionContribution, error) {
							return []domain.PermissionContribution{listGrant()}, nil
						}}, Sessions: mocks.Targets{Page: func(_ context.Context, q domain.SessionListQuery) (domain.SessionListPage, error) {
							gotQuery = q
							return domain.SessionListPage{Items: []domain.SessionSummary{row}, Total: 3, HasNext: true}, nil
						}}}
					}})
					if e != nil {
						t.Fatal(e)
					}
					input := session.ListInput{Field: field, Desc: desc, Limit: 1}
					first, e := svc.ListUserSessions(ctx, 42, input)
					if e != nil || first.NextCursor == "" {
						t.Fatal("first cursor", e)
					}
					input.Cursor = first.NextCursor
					input.Page = 2
					second, e := svc.ListUserSessions(ctx, 42, input)
					if e != nil || !second.HasPrev || gotQuery.After == nil || gotQuery.After.ID != row.ID || gotQuery.After.At.Valid == null {
						t.Fatal("cursor roundtrip", e)
					}
					for _, kind := range []string{"owner", "sort", "direction", "limit", "malformed", "unknown key", "wrong type", "invalid ID", "null nonnullable", "trailing JSON"} {
						t.Run(kind, func(t *testing.T) {
							q := input
							owner := uint64(42)
							switch kind {
							case "owner":
								owner = 43
							case "sort":
								q.Field = domain.SessionSortCreatedAt
								if field == domain.SessionSortCreatedAt {
									q.Field = domain.SessionSortExpiresAt
								}
							case "direction":
								q.Desc = !desc
							case "limit":
								q.Limit = 2
							case "malformed":
								q.Cursor = "!bad"
							default:
								data, _ := base64.RawURLEncoding.DecodeString(q.Cursor)
								var m map[string]any
								_ = json.Unmarshal(data, &m)
								switch kind {
								case "unknown key":
									m["password"] = "private"
								case "wrong type":
									m["id"] = 42
								case "invalid ID":
									m["id"] = "invalid"
								case "null nonnullable":
									if field == domain.SessionSortLastSeenAt {
										return
									}
									m["at"] = nil
								}
								data, _ = json.Marshal(m)
								if kind == "trailing JSON" {
									data = append(data, []byte(" {}")...)
								}
								q.Cursor = base64.RawURLEncoding.EncodeToString(data)
							}
							before := len(tx.Calls)
							r, e := svc.ListUserSessions(ctx, owner, q)
							if r != nil || !errors.Is(e, common.ErrBadRequest) || len(tx.Calls) != before {
								t.Fatal("mismatched cursor admitted", e)
							}
						})
					}
				})
			}
		}
	}
}
