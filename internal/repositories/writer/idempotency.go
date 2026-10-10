package writer

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type idempotencyStore struct{ tx domain.Transaction }
type idempotencyReplacement struct {
	RequestHash  []byte    `sql:"column:request_hash"`
	SafeResponse string    `sql:"column:safe_response"`
	CreatedAt    time.Time `sql:"column:created_at"`
	ExpiresAt    time.Time `sql:"column:expires_at"`
}

func (idempotencyReplacement) TableName() string { return "idempotency_records" }
func NewIdempotencyStore(tx domain.Transaction) domain.IdempotencyStore {
	return &idempotencyStore{tx: tx}
}

// The caller must hold the matching scope lock throughout lookup and Save.
func (s *idempotencyStore) Lookup(scope domain.IdempotencyScope) (*domain.IdempotencyRecord, error) {
	if s.tx == nil {
		return nil, common.ErrInternalServerError
	}
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	var record domain.IdempotencyRecord
	err := s.tx.UseModel(domain.IdempotencyRecord{}).Where("actor_id = ? AND method = ? AND route_scope = ? AND key = ?", scope.ActorID, scope.Method, scope.Target, scope.Key).Limit(1).Scan(&record)
	err = helpers.HandleDBError(err)
	if errors.Is(err, auth.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *idempotencyStore) Save(scope domain.IdempotencyScope, hash []byte, result domain.SafeCommandResult, now time.Time) error {
	if s.tx == nil {
		return common.ErrInternalServerError
	}
	if len(hash) == 0 || now.IsZero() {
		return common.ErrBadRequest
	}
	if err := domain.ValidateRequestHash(hash); err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	previous, err := s.Lookup(scope)
	if err != nil {
		return err
	}
	// Never replace an unexpired record, even for the same payload.
	if previous != nil && now.Before(previous.ExpiresAt) {
		return common.ErrBadRequest
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return common.ErrBadRequest
	}
	record := domain.IdempotencyRecord{ActorID: scope.ActorID, Method: scope.Method, RouteScope: scope.Target, Key: scope.Key, RequestHash: append([]byte(nil), hash...), SafeResponse: string(payload), CreatedAt: now.UTC(), ExpiresAt: now.UTC().Add(domain.IdempotencyWindow)}
	if previous == nil {
		return helpers.HandleDBError(s.tx.Create(&record))
	}
	// Explicit composite WHERE prevents ORM's single-primary-key update fallback.
	patch := idempotencyReplacement{record.RequestHash, record.SafeResponse, record.CreatedAt, record.ExpiresAt}
	updated, err := s.tx.UseModel(&patch).Where("actor_id = ? AND method = ? AND route_scope = ? AND key = ? AND expires_at <= ?", scope.ActorID, scope.Method, scope.Target, scope.Key, now.UTC()).Updates()
	if err != nil {
		return helpers.HandleDBError(err)
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return common.ErrInternalServerError
	}
	return nil
}
