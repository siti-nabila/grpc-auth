package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
)

type AuditCursor struct {
	Version    int    `json:"v"`
	Method     string `json:"method"`
	Query      string `json:"query"`
	OccurredAt string `json:"occurred_at"`
	ID         string `json:"id"`
}

func (q AuditLogQuery) QueryHash() string {
	data, _ := json.Marshal(struct {
		ActorUserID                                *uint64
		ActorService, Action, TargetType, TargetID string
		OccurredFrom, OccurredBefore               *time.Time
	}{q.ActorUserID, q.ActorService, q.Action, q.TargetType, q.TargetID, q.OccurredFrom, q.OccurredBefore})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (q AuditLogQuery) Cursor() (*AuditCursor, error) {
	if q.LastID == "" {
		return nil, nil
	}
	if len(q.LastID) > 4096 {
		return nil, common.ErrBadRequest
	}
	b, err := base64.RawURLEncoding.DecodeString(q.LastID)
	if err != nil || base64.RawURLEncoding.EncodeToString(b) != q.LastID {
		return nil, common.ErrBadRequest
	}
	var c AuditCursor
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Version != 1 || c.Method != AuditLogMethod || c.Query != q.QueryHash() {
		return nil, common.ErrBadRequest
	}
	canonical, _ := json.Marshal(c)
	if !bytes.Equal(b, canonical) {
		return nil, common.ErrBadRequest
	}
	id, err := strconv.ParseInt(c.ID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != c.ID {
		return nil, common.ErrBadRequest
	}
	at, err := time.Parse(time.RFC3339Nano, c.OccurredAt)
	if err != nil || at.Year() < 1 || at.Year() > 9999 || at.UTC().Format(time.RFC3339Nano) != c.OccurredAt || at.Nanosecond()%1000 != 0 {
		return nil, common.ErrBadRequest
	}
	return &c, nil
}

func (q AuditLogQuery) NextCursor(item AuditLogItem) string {
	b, _ := json.Marshal(AuditCursor{Version: 1, Method: AuditLogMethod, Query: q.QueryHash(), OccurredAt: item.OccurredAt.UTC().Format(time.RFC3339Nano), ID: strconv.FormatUint(item.ID, 10)})
	return base64.RawURLEncoding.EncodeToString(b)
}
