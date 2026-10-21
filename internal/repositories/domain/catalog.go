package domain

import (
	"context"
	"slices"
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/orm/orm"
)

type Catalog string

const (
	RoleCatalog                  Catalog = "/role.v1.RoleService/ListRoles"
	PermissionCatalog            Catalog = "/permission.v1.PermissionService/ListPermissions"
	ApplicationCatalog           Catalog = "/application.v1.ApplicationService/ListApplications"
	UserRoleAssignmentCatalog    Catalog = "/user.v1.UserService/ListUserRoles"
	UserApplicationAccessCatalog Catalog = "/user.v1.UserService/ListUserApplicationAccess"
)

type CatalogField struct {
	Name, Kind           string
	Nullable, Searchable bool
}

func (c Catalog) Fields() []CatalogField {
	switch c {
	case UserApplicationAccessCatalog:
		return []CatalogField{{"application_id", "integer", false, false}, {"application_code", "text", false, true}, {"name", "text", false, true}, {"application_status", "text", false, false}, {"is_system", "bool", false, false}, {"status", "text", false, false}, {"granted_by", "integer", true, false}, {"granted_at", "time", false, false}, {"revoked_by", "integer", true, false}, {"revoked_at", "time", true, false}, {"revoke_reason", "text", true, false}}
	case UserRoleAssignmentCatalog:
		return []CatalogField{{"id", "integer", false, false}, {"role_id", "integer", false, false}, {"code", "text", false, true}, {"role_code", "integer", false, false}, {"name", "text", false, true}, {"status", "text", false, false}, {"role_status", "text", false, false}, {"application_id", "integer", false, false}, {"application_code", "text", false, true}, {"application_status", "text", false, false}, {"is_system", "bool", false, false}, {"application_access_status", "text", true, false}, {"granted_by", "integer", true, false}, {"granted_at", "time", true, false}, {"revoked_by", "integer", true, false}, {"revoked_at", "time", true, false}, {"revoke_reason", "text", true, false}}
	case RoleCatalog:
		return []CatalogField{{"id", "integer", false, false}, {"code", "text", false, true}, {"name", "text", false, true}, {"description", "text", true, true}, {"application_id", "integer", false, false}, {"application_code", "text", false, true}, {"status", "text", false, false}, {"is_system", "bool", false, false}, {"version", "integer", false, false}, {"role_code", "integer", false, false}, {"created_at", "time", true, false}, {"updated_at", "time", true, false}, {"affected_user_count", "integer", false, false}}
	case PermissionCatalog:
		return []CatalogField{{"id", "integer", false, false}, {"application_id", "integer", false, false}, {"application_code", "text", false, true}, {"code", "text", false, true}, {"description", "text", false, true}}
	case ApplicationCatalog:
		return []CatalogField{{"id", "integer", false, false}, {"code", "text", false, true}, {"name", "text", false, true}, {"status", "text", false, false}, {"is_system", "bool", false, false}}
	default:
		return nil
	}
}

func (c Catalog) Field(name string) (CatalogField, bool) {
	for _, f := range c.Fields() {
		if f.Name == name {
			return f, true
		}
	}
	return CatalogField{}, false
}

type CatalogQuery struct {
	TargetUserID    uint64
	Page, Limit     int
	Fields          []string
	Sort            []orm.SortField
	Search          *orm.SearchQuery
	ApplicationCode string
	Statuses        []string
	LastID          string
}

type CatalogReader interface {
	Roles(context.Context, CatalogQuery) (orm.PageData[RoleSummary], error)
	Permissions(context.Context, CatalogQuery) (orm.PageData[PermissionItem], error)
	Applications(context.Context, CatalogQuery) (orm.PageData[ApplicationItem], error)
}

func (c Catalog) Normalize(q CatalogQuery) (CatalogQuery, error) {
	if err := c.normalizePage(&q); err != nil {
		return q, err
	}
	if err := c.normalizeFields(&q); err != nil {
		return q, err
	}
	if err := c.normalizeSort(&q); err != nil {
		return q, err
	}
	if err := c.normalizeFilters(&q); err != nil {
		return q, err
	}
	if err := c.normalizeSearch(&q); err != nil {
		return q, err
	}
	if _, err := c.ParseCursor(q); err != nil {
		return q, err
	}
	return q, nil
}

func (c Catalog) normalizePage(q *CatalogQuery) error {
	if (c == UserRoleAssignmentCatalog || c == UserApplicationAccessCatalog) && ValidateManagementTarget(q.TargetUserID) != nil {
		return common.ErrBadRequest
	}
	if len(c.Fields()) == 0 || q.Page < 0 || q.Limit < 0 || q.Limit > 100 {
		return common.ErrBadRequest
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	return nil
}

func (c Catalog) normalizeFields(q *CatalogQuery) error {
	q.Fields = slices.Clone(q.Fields)
	seen := map[string]bool{}
	for _, name := range q.Fields {
		if _, ok := c.Field(name); !ok || seen[name] {
			return common.ErrBadRequest
		}
		seen[name] = true
	}
	if len(q.Fields) > 0 && !seen[c.IdentityField()] {
		q.Fields = append(q.Fields, c.IdentityField())
	}
	if c == UserRoleAssignmentCatalog && len(q.Fields) > 0 && !seen["role_id"] {
		q.Fields = append(q.Fields, "role_id")
	}
	return nil
}

func (c Catalog) normalizeSort(q *CatalogQuery) error {
	q.Sort = slices.Clone(q.Sort)
	seen := map[string]bool{}
	for _, s := range q.Sort {
		if _, ok := c.Field(s.Field); !ok || seen[s.Field] {
			return common.ErrBadRequest
		}
		seen[s.Field] = true
	}
	if !seen[c.IdentityField()] {
		q.Sort = append(q.Sort, orm.SortField{Field: c.IdentityField()})
	}
	return nil
}

func (c Catalog) normalizeFilters(q *CatalogQuery) error {
	if q.ApplicationCode != "" && (!applicationCodeValid(q.ApplicationCode) || c == ApplicationCatalog) {
		return common.ErrBadRequest
	}
	q.Statuses = slices.Clone(q.Statuses)
	for _, status := range q.Statuses {
		terminal := "DISABLED"
		if c == UserRoleAssignmentCatalog || c == UserApplicationAccessCatalog {
			terminal = "REVOKED"
		}
		if c == PermissionCatalog || status != "ACTIVE" && status != terminal {
			return common.ErrBadRequest
		}
	}
	slices.Sort(q.Statuses)
	q.Statuses = slices.Compact(q.Statuses)
	if len(q.Statuses) == 0 {
		q.Statuses = nil
	}
	return nil
}

func (c Catalog) normalizeSearch(q *CatalogQuery) error {
	if q.Search != nil {
		s := *q.Search
		s.Fields = slices.Clone(s.Fields)
		s.Keyword = strings.TrimSpace(s.Keyword)
		if !userText(s.Keyword, 0, 200) {
			return common.ErrBadRequest
		}
		if s.Mode == "" {
			s.Mode = orm.SearchModeContains
		}
		if s.Mode != orm.SearchModeContains && s.Mode != orm.SearchModePrefix {
			return common.ErrBadRequest
		}
		seen := map[string]bool{}
		for _, name := range s.Fields {
			f, ok := c.Field(name)
			if !ok || !f.Searchable || seen[name] {
				return common.ErrBadRequest
			}
			seen[name] = true
		}
		if len(s.Fields) == 0 {
			s.Fields = []string{"code", "name"}
			if c == PermissionCatalog {
				s.Fields = []string{"code", "description"}
			}
			if c == UserApplicationAccessCatalog {
				s.Fields = []string{"application_code", "name"}
			}
		}
		slices.Sort(s.Fields)
		q.Search = &s
		if s.Keyword == "" {
			q.Search = nil
		}
	}
	return nil
}

func (q CatalogQuery) Projects(name string) bool {
	return len(q.Fields) == 0 || slices.Contains(q.Fields, name)
}

// IdentityField is unique within a catalog query's fixed target scope.
func (c Catalog) IdentityField() string {
	if c == UserApplicationAccessCatalog {
		return "application_id"
	}
	return "id"
}
