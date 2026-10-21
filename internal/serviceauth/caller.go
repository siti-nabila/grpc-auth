package serviceauth

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
)

// RestORCServiceID is the only caller trusted to forward the client IP.
const RestORCServiceID = "rest-orc"

type callerKey struct{}

// Caller only exposes server-owned identity and exact namespace authority.
// Its state cannot be mutated through configuration slices or a returned value.
type Caller struct {
	id         string
	namespaces map[string]bool
}

func (c Caller) ID() string { return c.id }

func (c Caller) AuthorizeNamespace(namespace string) error {
	if c.id == "" || !c.namespaces[namespace] {
		return common.ErrForbidden
	}
	return nil
}

func CallerFromContext(ctx context.Context) (Caller, error) {
	caller, ok := ctx.Value(callerKey{}).(Caller)
	if !ok || caller.id == "" {
		return Caller{}, auth.ErrServiceAuthenticationFailed
	}
	return caller, nil
}
