//go:build postgres

package test_scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	commonerror "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	data "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func PersistenceCommands() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "committed management result replays before stale version and conflicts without side effects", Run: persistentReplay},
		{Name: "24 hour boundary replaces under exclusion and no op replay emits no event", Run: expiryAndNoOp},
		{Name: "concurrent claims use separate backends and one atomic result", Run: concurrentIdempotency},
		{Name: "failure at each production repository rolls back all four writes", Run: persistenceRollback},
		{Name: "explicit commit then denial preserves required security fixture writes", Run: commitThenDenial},
	}
}

type managementCommand struct {
	scope        domain.IdempotencyScope
	payload      common.FingerprintPayload
	name         string
	now          time.Time
	eventID      string
	noOp         bool
	deny         bool
	fail         string
	beforeCommit func(domain.Transaction) error
}

func fixtureCommand() managementCommand {
	name := "managed fixture"
	return managementCommand{scope: data.Scope(), payload: common.FingerprintPayload{Name: &name, Presence: []string{"name"}, ExpectedVersion: 1}, name: name, now: data.PersistenceTime, eventID: data.EventID}
}

var fixtureVersionConflict = errors.New("fixture expected version conflict")

// This verified synthetic consumer is not a public management or refresh RPC.
// Its principal/permission gate is explicit and evaluated on every invocation.
func executeManagement(ctx context.Context, provider domain.TransactionProvider, cmd managementCommand, authorized bool) (result domain.SafeCommandResult, err error) {
	if !authorized {
		return result, commonerror.ErrForbidden
	}
	key, err := common.IdempotencyLockKey(cmd.scope)
	if err != nil {
		return result, err
	}
	hash, err := common.Fingerprint(cmd.scope, cmd.payload)
	if err != nil {
		return result, err
	}
	tx, err := provider.Begin(ctx)
	if err != nil {
		return result, err
	}
	owner := common.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = common.AcquireLocks(owner, user.ErrResourceBusy, key, common.UserLockKey(1)); err != nil {
		return result, err
	}
	store := writer.NewIdempotencyStore(tx)
	previous, err := store.Lookup(cmd.scope)
	if err != nil {
		return result, err
	}
	replayed, err := common.Replay(previous, hash, cmd.now)
	if err != nil {
		return result, err
	}
	if replayed != nil {
		return *replayed, nil
	}
	var counters fixtureCounters
	if err = tx.UseModel(fixtureCounters{}).Where("id = ?", 1).Limit(1).Scan(&counters); err != nil {
		return result, err
	}
	if counters.Version != cmd.payload.ExpectedVersion {
		return result, fixtureVersionConflict
	}
	profile, err := reader.NewProfileReaderInTransaction(tx).GetByUserId(1)
	if err != nil {
		return result, err
	}
	if !cmd.noOp {
		if err = writer.NewProfileWriterInTransaction(tx).Update(&domain.UpdateProfileRequest{Id: profile.Id, Name: cmd.name}); err != nil {
			return result, err
		}
		if cmd.fail == "mutation" {
			return result, errors.New("controlled failure after mutation")
		}
		counters.Version++
		patch := fixtureVersionUpdate{ID: 1, Version: counters.Version}
		if err = tx.Update(&patch); err != nil {
			return result, err
		}
		if cmd.deny {
			security := fixtureSecurityUpdate{ID: 1, Status: "SUSPENDED", AuthVersion: counters.AuthVersion + 1}
			if err = tx.Update(&security); err != nil {
				return result, err
			}
		}
	}
	audit := data.Audit()
	audit.OccurredAt = cmd.now
	audit.Before = &domain.SafeSnapshot{Version: cmd.payload.ExpectedVersion}
	audit.After = &domain.SafeSnapshot{Version: counters.Version, NoOp: cmd.noOp}
	if cmd.noOp {
		audit.Action = "NO_OP"
	}
	if cmd.deny {
		audit.Action = "USER_STATUS_CHANGED"
		audit.After.Status = "SUSPENDED"
		audit.After.ReasonCode = "ACCESS_REVOKED"
	}
	if cmd.fail == "audit" {
		audit.Actor, _ = domain.ActorForVerifiedUser(999999)
	} // actual FK failure
	if err = writer.NewAuditWriter(tx).Create(audit); err != nil {
		return result, err
	}
	if !cmd.noOp {
		event := data.Event()
		event.EventID = cmd.eventID
		event.OccurredAt = cmd.now
		event.AggregateVersion = counters.Version
		if cmd.deny {
			event.EventType = "iam.user.status_changed.v1"
			event.Data.Status = "SUSPENDED"
			event.Data.ReasonCode = "ACCESS_REVOKED"
		}
		if cmd.fail == "outbox" {
			// First write is valid. The second required write fails its actual PK.
			if err = writer.NewOutboxWriter(tx).Create(event); err != nil {
				return result, err
			}
		}
		if err = writer.NewOutboxWriter(tx).Create(event); err != nil {
			return result, err
		}
	}
	result = domain.SafeCommandResult{UserID: 1, Version: counters.Version, Changed: !cmd.noOp, Name: &cmd.name}
	if err = store.Save(cmd.scope, hash, result, cmd.now); err != nil {
		return domain.SafeCommandResult{}, err
	}
	if cmd.fail == "idempotency" {
		// Bypass this fixture's own contract after valid Save to exercise DB PK
		// failure and transaction rollback, never a second success record.
		duplicate := domain.IdempotencyRecord{ActorID: cmd.scope.ActorID, Method: cmd.scope.Method, RouteScope: cmd.scope.Target, Key: cmd.scope.Key, RequestHash: hash, SafeResponse: `{"changed":true}`, CreatedAt: cmd.now, ExpiresAt: cmd.now.Add(domain.IdempotencyWindow)}
		if err = tx.Create(&duplicate); err != nil {
			return domain.SafeCommandResult{}, err
		}
	}
	if cmd.beforeCommit != nil {
		if err = cmd.beforeCommit(tx); err != nil {
			return domain.SafeCommandResult{}, err
		}
	}
	if err = owner.Commit(); err != nil {
		return domain.SafeCommandResult{}, err
	}
	if cmd.deny {
		return domain.SafeCommandResult{}, commonerror.ErrUnauthorized
	}
	return result, nil
}

type fixtureVersionUpdate struct {
	ID      uint64 `sql:"column:id;primaryKey"`
	Version int64  `sql:"column:version"`
}

func (fixtureVersionUpdate) TableName() string { return "auth" }

type fixtureSecurityUpdate struct {
	ID          uint64 `sql:"column:id;primaryKey"`
	Status      string `sql:"column:status"`
	AuthVersion int64  `sql:"column:auth_version"`
}

func (fixtureSecurityUpdate) TableName() string { return "auth" }

type fixtureCounters struct {
	ID          uint64 `sql:"column:id"`
	Version     int64  `sql:"column:version"`
	AuthVersion int64  `sql:"column:auth_version"`
}

func (fixtureCounters) TableName() string { return "auth" }

func persistenceCounts(t *testing.T, ctx context.Context, f *fixtures.Schema, audit, outbox, idem int, name string, version int64, authVersion ...int64) {
	t.Helper()
	var a, o, i int
	var v, av int64
	var n string
	err := f.DB.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM audit_logs),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM idempotency_records),p.name,a.version,a.auth_version FROM auth a JOIN profile p ON p.user_id=a.id WHERE a.id=1").Scan(&a, &o, &i, &n, &v, &av)
	wantAuth := int64(1)
	if len(authVersion) > 0 {
		wantAuth = authVersion[0]
	}
	if err != nil || a != audit || o != outbox || i != idem || n != name || v != version || av != wantAuth {
		t.Fatalf("atomic state mismatch: audit=%d outbox=%d result=%d version=%d auth_version=%d err=%v", a, o, i, v, av, err)
	}
	var unsafe, published int
	if err = f.DB.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NOT NULL OR attempts <> 0`).Scan(&published); err != nil || published != 0 {
		t.Fatal("pending event marked delivered")
	}
	if err = f.DB.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT safe_before::text||COALESCE(safe_after::text,'') AS body FROM audit_logs UNION ALL SELECT payload::text FROM outbox_events UNION ALL SELECT safe_response::text FROM idempotency_records) s WHERE body ~ '"(password|password_hash|access_token|refresh_token|authorization)"'`).Scan(&unsafe); err != nil || unsafe != 0 {
		t.Fatal("credential field persisted")
	}
}

func persistentReplay(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	provider := productionProvider(f)
	cmd := fixtureCommand()
	first, err := executeManagement(ctx, provider, cmd, true)
	if err != nil || !first.Changed || first.Version != 2 {
		t.Fatal("first command failed", err)
	}
	// The first response may have been lost. Original expected version is now
	// stale, but lookup/replay precedes that check on the next fresh transaction.
	second, err := executeManagement(ctx, provider, cmd, true)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("lost-response replay differs", err)
	}
	if _, err = executeManagement(ctx, provider, cmd, false); !errors.Is(err, commonerror.ErrForbidden) {
		t.Fatal("replay bypassed current permission")
	}
	otherName := "different"
	changed := cmd
	changed.name = otherName
	changed.payload.Name = &otherName
	if _, err = executeManagement(ctx, provider, changed, true); !errors.Is(err, user.ErrIdempotencyConflict) {
		t.Fatal("different payload not conflict", err)
	}
	changed = cmd
	changed.scope.Target = "user/1/application/1"
	if _, err = executeManagement(ctx, provider, changed, true); !errors.Is(err, fixtureVersionConflict) {
		t.Fatal("different target incorrectly replayed", err)
	}
	persistenceCounts(t, ctx, f, 1, 1, 1, cmd.name, 2)
	changed.payload.ExpectedVersion = 2
	changed.eventID = "10000000-0000-4000-8000-000000000002"
	third, err := executeManagement(ctx, provider, changed, true)
	if err != nil || third.Version != 3 {
		t.Fatal("independent target scope failed", err)
	}
	second, err = executeManagement(ctx, provider, cmd, true)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("other target overwrote original result")
	}
	persistenceCounts(t, ctx, f, 2, 2, 2, cmd.name, 3)
	var payload string
	var expiry, created time.Time
	if err = f.DB.QueryRowContext(ctx, "SELECT safe_response::text,created_at,expires_at FROM idempotency_records WHERE route_scope=$1", cmd.scope.Target).Scan(&payload, &created, &expiry); err != nil || expiry.Sub(created) != domain.IdempotencyWindow {
		t.Fatal("retention not exactly 24h")
	}
	var saved domain.SafeCommandResult
	if err = json.Unmarshal([]byte(payload), &saved); err != nil || !reflect.DeepEqual(saved, first) {
		t.Fatal("saved result differs")
	}
}

func expiryAndNoOp(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	provider := productionProvider(f)
	cmd := fixtureCommand()
	cmd.noOp = true
	cmd.name = "Fixture user"
	cmd.payload.Name = &cmd.name
	first, err := executeManagement(ctx, provider, cmd, true)
	if err != nil || first.Changed || first.Version != 1 {
		t.Fatal("no op failed", err)
	}
	second, err := executeManagement(ctx, provider, cmd, true)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("no op replay failed")
	}
	persistenceCounts(t, ctx, f, 1, 0, 1, "Fixture user", 1)
	cmd.now = cmd.now.Add(domain.IdempotencyWindow - time.Nanosecond)
	if _, err = executeManagement(ctx, provider, cmd, true); err != nil {
		t.Fatal(err)
	}
	persistenceCounts(t, ctx, f, 1, 0, 1, "Fixture user", 1)
	cmd.now = data.PersistenceTime.Add(domain.IdempotencyWindow)
	cmd.noOp = false
	cmd.name = "after expiry"
	cmd.payload.Name = &cmd.name
	result, err := executeManagement(ctx, provider, cmd, true)
	if err != nil || !result.Changed || result.Version != 2 {
		t.Fatal("expired replacement failed", err)
	}
	persistenceCounts(t, ctx, f, 2, 1, 1, cmd.name, 2)
	var created, expiry time.Time
	if err = f.DB.QueryRowContext(ctx, "SELECT created_at,expires_at FROM idempotency_records").Scan(&created, &expiry); err != nil || !created.Equal(cmd.now) || !expiry.Equal(cmd.now.Add(domain.IdempotencyWindow)) {
		t.Fatal("replacement window incorrect")
	}
}

func concurrentIdempotency(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	provider := productionProvider(f)
	cmd := fixtureCommand()
	ready := make(chan int, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	cmd.beforeCommit = func(tx domain.Transaction) error {
		var row struct {
			PID int `sql:"column:pid"`
		}
		if err := tx.UseModel(domain.Auth{}).Select("pg_backend_pid() AS pid").Limit(1).Scan(&row); err != nil {
			return err
		}
		ready <- row.PID
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	released := false
	finished := false
	defer func() {
		if !released {
			close(release)
		}
		if !finished {
			select {
			case <-done:
			case <-ctx.Done():
			}
		}
	}()
	go func() { _, err := executeManagement(ctx, provider, cmd, true); done <- err }()
	var pidA int
	select {
	case pidA = <-ready:
	case err := <-done:
		finished = true
		t.Fatal("claim exited before barrier", err)
	case <-ctx.Done():
		t.Fatal("claim worker timeout")
	}
	persistenceCounts(t, ctx, f, 0, 0, 0, "Fixture user", 1)
	contender := &observedProvider{inner: provider}
	cmd.beforeCommit = nil
	if _, err := executeManagement(ctx, contender, cmd, true); !errors.Is(err, user.ErrResourceBusy) || contender.pid == pidA {
		t.Fatal("concurrent claim did not exclude distinct backend", err)
	}
	close(release)
	released = true
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("claim did not finish")
	}
	if _, err := executeManagement(ctx, provider, cmd, true); err != nil {
		t.Fatal("claim retry failed", err)
	}
	persistenceCounts(t, ctx, f, 1, 1, 1, cmd.name, 2)
}

func persistenceRollback(t *testing.T) {
	for _, stage := range []string{"mutation", "audit", "outbox", "idempotency"} {
		t.Run(stage, func(t *testing.T) {
			ctx, _, f := dedicatedFixture(t)
			provider := productionProvider(f)
			cmd := fixtureCommand()
			cmd.fail = stage
			if _, err := executeManagement(ctx, provider, cmd, true); err == nil {
				t.Fatal("required failure accepted")
			}
			persistenceCounts(t, ctx, f, 0, 0, 0, "Fixture user", 1)
			cmd.fail = ""
			if _, err := executeManagement(ctx, provider, cmd, true); err != nil {
				t.Fatal("failed command leaked lock/result", err)
			}
			persistenceCounts(t, ctx, f, 1, 1, 1, cmd.name, 2)
		})
	}
}

func commitThenDenial(t *testing.T) {
	for _, fail := range []string{"", "audit", "outbox"} {
		t.Run(fmt.Sprintf("required failure %q", fail), func(t *testing.T) {
			ctx, _, f := dedicatedFixture(t)
			cmd := fixtureCommand()
			cmd.deny = true
			cmd.fail = fail
			cmd.name = "security fixture committed"
			cmd.payload.Name = &cmd.name
			_, err := executeManagement(ctx, productionProvider(f), cmd, true)
			if fail == "" {
				if !errors.Is(err, commonerror.ErrUnauthorized) {
					t.Fatal("denial lost", err)
				}
				persistenceCounts(t, ctx, f, 1, 1, 1, cmd.name, 2, 2)
			} else {
				if err == nil || errors.Is(err, commonerror.ErrUnauthorized) {
					t.Fatal("failed security write claimed revocation")
				}
				persistenceCounts(t, ctx, f, 0, 0, 0, "Fixture user", 1)
			}
			var status string
			if err = f.DB.QueryRowContext(ctx, "SELECT status FROM auth WHERE id=1").Scan(&status); err != nil {
				t.Fatal(err)
			}
			want := "ACTIVE"
			if fail == "" {
				want = "SUSPENDED"
			}
			if status != want {
				t.Fatal("security status did not follow terminal outcome")
			}
			// Controlled status/auth_version writes prove terminal ownership. No
			// session store, JWT enforcement or RefreshSession is implemented here.
			if f.DB.Stats().InUse != 0 {
				t.Fatal("security outcome leaked handle")
			}
		})
	}
}
