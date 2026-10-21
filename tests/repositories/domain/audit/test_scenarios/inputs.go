package test_scenarios

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"math"
	"strings"
	"testing"
	"time"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "minimal exact query normalization and invalid input", Run: inputs}, {Name: "fixed BIGINT canonical cursor scope and rejection", Run: cursors}, {Name: "typed historical snapshots preserve null and reject unsafe storage", Run: rows}}
}
func inputs(t *testing.T) {
	zero, large := uint64(0), uint64(math.MaxInt32)+1
	now := fixtures.ServiceTime
	before := now.Add(time.Hour)
	for name, q := range map[string]domain.AuditLogQuery{
		"negative page": {Page: -1}, "negative limit": {Limit: -1}, "limit overflow": {Limit: 101}, "unsupported": {Unsupported: true}, "zero actor": {ActorUserID: &zero}, "large actor": {ActorUserID: &large}, "long service": {ActorService: strings.Repeat("a", 101)}, "long action": {Action: strings.Repeat("a", 101)}, "long type": {TargetType: strings.Repeat("a", 51)}, "long target": {TargetType: "user", TargetID: strings.Repeat("a", 101)}, "target missing type": {TargetID: "7"}, "control": {Action: "a\nb"}, "invalid UTF8": {Action: string([]byte{255})}, "bad year": {OccurredFrom: new(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))}, "equal bounds": {OccurredFrom: &now, OccurredBefore: &now}, "reversed bounds": {OccurredFrom: &before, OccurredBefore: &now}, "long cursor": {LastID: strings.Repeat("a", 4097)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := q.Normalized(); !errors.Is(err, common.ErrBadRequest) {
				t.Fatal(err)
			}
		})
	}
	id := uint64(7)
	from := now.In(time.FixedZone("offset", 7*3600))
	q, err := (domain.AuditLogQuery{ActorUserID: &id, ActorService: " svc ", Action: " unknown%_ ", TargetType: " rpc ", TargetID: " /service/method ", OccurredFrom: &from, OccurredBefore: &before}).Normalized()
	if err != nil || q.Page != 1 || q.Limit != 20 || q.ActorService != "svc" || q.Action != "unknown%_" || q.TargetType != "rpc" || q.OccurredFrom.Location() != time.UTC || q.ActorUserID == &id {
		t.Fatal(q, err)
	}
	if _, err = (domain.AuditLogQuery{TargetType: "unregistered", ActorService: " ", Page: math.MaxInt32, Limit: 100}).Normalized(); err != nil {
		t.Fatal(err)
	}
}
func cursors(t *testing.T) {
	q, _ := (domain.AuditLogQuery{ActorService: "svc"}).Normalized()
	for _, id := range []uint64{math.MaxInt32 + 1, math.MaxInt64} {
		q.LastID = q.NextCursor(domain.AuditLogItem{ID: id, OccurredAt: fixtures.ServiceTime})
		c, err := q.Cursor()
		if err != nil || c.ID == "" {
			t.Fatal(c, err)
		}
		q.Page = 9
		q.Limit = 100
		if _, err = q.Normalized(); err != nil {
			t.Fatal("metadata should not invalidate cursor", err)
		}
	}
	valid := q.LastID
	for _, name := range []string{"method", "filter", "version", "zero", "negative", "overflow", "noncanonical ID", "timestamp", "nanoseconds", "timezone", "unknown", "trailing", "space", "numeric", "padding"} {
		t.Run(name, func(t *testing.T) {
			copy := q
			copy.LastID = valid
			b, _ := base64.RawURLEncoding.DecodeString(valid)
			var c domain.AuditCursor
			_ = json.Unmarshal(b, &c)
			switch name {
			case "method":
				c.Method = "/other/method"
			case "filter":
				copy.Action = "changed"
			case "version":
				c.Version = 2
			case "zero":
				c.ID = "0"
			case "negative":
				c.ID = "-1"
			case "overflow":
				c.ID = "9223372036854775808"
			case "noncanonical ID":
				c.ID = "01"
			case "timestamp":
				c.OccurredAt = "invalid"
			case "nanoseconds":
				c.OccurredAt = "2026-10-18T00:00:00.000000001Z"
			case "timezone":
				c.OccurredAt = "2026-10-18T00:00:00+00:00"
			}
			b, _ = json.Marshal(c)
			switch name {
			case "unknown":
				b = append(b[:len(b)-1], []byte(`,"extra":1}`)...)
			case "trailing":
				b = append(b, []byte(`{}`)...)
			case "space":
				b = append([]byte(" "), b...)
			}
			copy.LastID = base64.RawURLEncoding.EncodeToString(b)
			if name == "numeric" {
				copy.LastID = "7"
			}
			if name == "padding" {
				copy.LastID += "="
			}
			if _, err := copy.Cursor(); !errors.Is(err, common.ErrBadRequest) {
				t.Fatal(name, err)
			}
		})
	}
}
func rows(t *testing.T) {
	id := uint64(7)
	svc := "service"
	empty := "{}"
	blank := ""
	base := domain.AuditRecord{ID: math.MaxInt64, ActorUserID: &id, ActorService: &svc, Action: "NO_OP", TargetType: "user", TargetID: "7", OccurredAt: fixtures.ServiceTime, SafeAfter: &empty, Reason: &blank}
	item, err := base.SafeItem()
	if err != nil || item.ID != math.MaxInt64 || item.Before != nil || item.After == nil || item.ActorUserID == nil || item.ActorService == nil || item.Reason == nil {
		t.Fatal(item, err)
	}
	for _, raw := range []string{`null`, `[]`, `"text"`, `{"access_token":"synthetic"}`, `{}`, `{"name":"","description":"","address":"","phone":"","role_ids":[2],"permission_codes":["iam.audit.read"],"session_id":"00000000-0000-4000-8000-000000000001"}`, `{"version":-1}`, `{"role_ids":[0]}`, `{"session_id":"bad"}`, `{bad`, `{} {}`} {
		t.Run(raw, func(t *testing.T) {
			r := base
			r.SafeBefore = &raw
			v, err := r.SafeItem()
			valid := raw == `{}` || strings.HasPrefix(raw, `{"name"`)
			if valid && (err != nil || v.Before == nil) {
				t.Fatal(v, err)
			}
			if !valid && !errors.Is(err, common.ErrDependencyUnavailable) {
				t.Fatal(v, err)
			}
		})
	}
	for _, name := range []string{"zero ID", "negative ID", "no actor", "bad actor", "empty service", "invalid action", "invalid time"} {
		t.Run(name, func(t *testing.T) {
			r := base
			switch name {
			case "zero ID":
				r.ID = 0
			case "negative ID":
				r.ID = -1
			case "no actor":
				r.ActorService = nil
				r.ActorUserID = nil
			case "bad actor":
				id := uint64(math.MaxInt32) + 1
				r.ActorUserID = &id
			case "empty service":
				r.ActorService = &blank
			case "invalid action":
				r.Action = ""
			case "invalid time":
				r.OccurredAt = time.Time{}
			}
			if _, err := r.SafeItem(); !errors.Is(err, common.ErrDependencyUnavailable) {
				t.Fatal(err)
			}
		})
	}
}
