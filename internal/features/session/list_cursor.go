package session

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"io"
	"math"
	"time"
)

type listCursor struct {
	Version int                     `json:"v"`
	Owner   uint64                  `json:"owner"`
	Field   domain.SessionSortField `json:"field"`
	Desc    bool                    `json:"desc"`
	Limit   int                     `json:"limit"`
	At      *time.Time              `json:"at"`
	ID      string                  `json:"id"`
}

func normalizeList(target uint64, r ListInput) (ListInput, domain.SessionListQuery, error) {
	if r.Page < 0 || r.Page > math.MaxInt32 || r.Limit < 0 {
		return r, domain.SessionListQuery{}, common.ErrBadRequest
	}
	if r.Page == 0 {
		r.Page = 1
	}
	if r.Limit == 0 {
		r.Limit = 20
	}
	if r.Limit > 100 {
		r.Limit = 100
	}
	if r.Field == "" {
		r.Field = domain.SessionSortCreatedAt
		r.Desc = true
	}
	q := domain.SessionListQuery{UserID: target, Field: r.Field, Desc: r.Desc, Limit: r.Limit}
	if r.Cursor != "" {
		if len(r.Cursor) > 2048 {
			return r, q, common.ErrBadRequest
		}
		data, err := base64.RawURLEncoding.Strict().DecodeString(r.Cursor)
		if err != nil || base64.RawURLEncoding.EncodeToString(data) != r.Cursor {
			return r, q, common.ErrBadRequest
		}
		var c listCursor
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Version != 1 || c.Owner != target || c.Field != r.Field || c.Desc != r.Desc || c.Limit != r.Limit {
			return r, q, common.ErrBadRequest
		}
		canonical, err := json.Marshal(c)
		if err != nil || !bytes.Equal(data, canonical) {
			return r, q, common.ErrBadRequest
		}
		at := sql.NullTime{}
		if c.At != nil {
			at = sql.NullTime{Time: c.At.UTC(), Valid: true}
		}
		q.After = &domain.SessionListCursor{At: at, ID: c.ID}
	}
	return r, q, q.Validate()
}

func nextSessionCursor(q domain.SessionListQuery, s domain.SessionSummary) (string, error) {
	c := listCursor{Version: 1, Owner: q.UserID, Field: q.Field, Desc: q.Desc, Limit: q.Limit, ID: s.ID}
	switch q.Field {
	case domain.SessionSortCreatedAt:
		at := s.CreatedAt.UTC()
		c.At = &at
	case domain.SessionSortExpiresAt:
		at := s.ExpiresAt.UTC()
		c.At = &at
	case domain.SessionSortLastSeenAt:
		if s.LastSeenAt.Valid {
			at := s.LastSeenAt.Time.UTC()
			c.At = &at
		}
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", common.ErrInternalServerError
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
