package domain

import (
	"strings"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type EvaluateAccessInput struct {
	ApplicationCode string
	PermissionCode  string
}

func (r EvaluateAccessInput) Normalized() EvaluateAccessInput {
	r.ApplicationCode = strings.TrimSpace(r.ApplicationCode)
	r.PermissionCode = strings.TrimSpace(r.PermissionCode)
	return r
}

func (r EvaluateAccessInput) Validate() error {
	r = r.Normalized()
	if !applicationCodeValid(r.ApplicationCode) || !userText(r.PermissionCode, 1, 150) || !permissionCodePattern.MatchString(r.PermissionCode) || !strings.HasPrefix(r.PermissionCode, r.ApplicationCode+".") {
		return common.ErrBadRequest
	}
	return nil
}

// AccessDecision describes one evaluation; Version is not a policy revision.
type AccessDecision struct {
	Allowed     bool
	Reason      types.AccessDecisionReason
	UserID      uint64
	SessionID   string
	Version     int64
	EvaluatedAt time.Time
}
