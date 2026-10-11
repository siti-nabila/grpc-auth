package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func readUserCollections(ctx context.Context, db domain.QueryReader, rows []domain.UserSearchRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	byID := make(map[int64]int, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].AuthID)
		byID[rows[i].AuthID] = i
		rows[i].Roles = []domain.UserListRole{}
		rows[i].ApplicationAccess = []domain.UserListApplication{}
	}
	var roles []domain.UserListRole
	err := db.UseModel(domain.UserListRole{}).WithContext(ctx).Select("ur.user_id", "r.id AS role_id", "r.code", "r.role_name AS name", "ur.status", "r.status AS role_status").Join("role r", "r.id = ur.role_id").WhereIn("ur.user_id", ids).Where("ur.status = ? AND r.status = ?", types.AssignmentActive, types.RoleActive).OrderBy("ur.user_id ASC", "r.role_name ASC", "r.id ASC").Scan(&roles)
	if err != nil {
		return userPageError(err)
	}
	for _, role := range roles {
		i, ok := byID[role.UserID]
		if !ok {
			return common.ErrDependencyUnavailable
		}
		rows[i].Roles = append(rows[i].Roles, role)
	}
	var apps []domain.UserListApplication
	err = db.UseModel(domain.UserListApplication{}).WithContext(ctx).Select("ua.user_id", "app.id AS application_id", "app.code", "app.name", "ua.status", "app.status AS application_status").Join("applications app", "app.id = ua.application_id").WhereIn("ua.user_id", ids).Where("ua.status = ? AND app.status = ?", types.ApplicationAccessActive, types.ApplicationActive).OrderBy("ua.user_id ASC", "app.name ASC", "app.id ASC").Scan(&apps)
	if err != nil {
		return userPageError(err)
	}
	for _, app := range apps {
		i, ok := byID[app.UserID]
		if !ok {
			return common.ErrDependencyUnavailable
		}
		rows[i].ApplicationAccess = append(rows[i].ApplicationAccess, app)
	}
	return nil
}
