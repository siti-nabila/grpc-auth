package domain

func (r RoleSummary) Project(q CatalogQuery) RoleSummary {
	out := RoleSummary{ID: r.ID}
	if q.Projects("code") {
		out.Code = r.Code
	}
	if q.Projects("name") {
		out.Name = r.Name
	}
	if q.Projects("description") {
		out.Description = r.Description
	}
	if q.Projects("application_id") {
		out.ApplicationID = r.ApplicationID
	}
	if q.Projects("application_code") {
		out.ApplicationCode = r.ApplicationCode
	}
	if q.Projects("status") {
		out.Status = r.Status
	}
	if q.Projects("is_system") {
		out.IsSystem = r.IsSystem
	}
	if q.Projects("version") {
		out.Version = r.Version
	}
	if q.Projects("role_code") {
		out.NumericCode = r.NumericCode
	}
	if q.Projects("created_at") {
		out.CreatedAt = r.CreatedAt
	}
	if q.Projects("updated_at") {
		out.UpdatedAt = r.UpdatedAt
	}
	if q.Projects("affected_user_count") {
		out.AffectedUserCount = r.AffectedUserCount
	}
	return out
}

func (r PermissionItem) Project(q CatalogQuery) PermissionItem {
	out := PermissionItem{ID: r.ID}
	if q.Projects("code") {
		out.Code = r.Code
	}
	if q.Projects("application_id") {
		out.ApplicationID = r.ApplicationID
	}
	if q.Projects("application_code") {
		out.ApplicationCode = r.ApplicationCode
	}
	if q.Projects("description") {
		out.Description = r.Description
	}
	return out
}

func (r ApplicationItem) Project(q CatalogQuery) ApplicationItem {
	out := ApplicationItem{ID: r.ID}
	if q.Projects("code") {
		out.Code = r.Code
	}
	if q.Projects("name") {
		out.Name = r.Name
	}
	if q.Projects("status") {
		out.Status = r.Status
	}
	if q.Projects("is_system") {
		out.IsSystem = r.IsSystem
	}
	return out
}
