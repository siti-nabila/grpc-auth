package sessions

import (
	"context"
	"errors"
)

const UserSessionKey contextKey = "userSession"

type (
	contextKey  string
	UserSession struct {
		UserId uint64
	}
)

func GetUserSession(ctx context.Context) (*UserSession, error) {
	session, ok := ctx.Value(UserSessionKey).(*UserSession)
	if !ok {
		return nil, errors.New("user session not found in context")
	}
	return session, nil
}
