package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/orm/orm"
)

type CatalogValue struct {
	Kind  string  `json:"kind"`
	Value *string `json:"value"`
}
type CatalogCursor struct {
	Version int            `json:"v"`
	Method  Catalog        `json:"method"`
	Query   string         `json:"query"`
	Values  []CatalogValue `json:"values"`
}

func (c Catalog) QueryHash(q CatalogQuery) string {
	b, _ := json.Marshal(struct {
		Method       Catalog
		Sort         []orm.SortField
		Application  string
		Statuses     []string
		Search       *orm.SearchQuery
		TargetUserID uint64 `json:"target_user_id,omitempty"`
	}{c, q.Sort, q.ApplicationCode, q.Statuses, q.Search, q.TargetUserID})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (c Catalog) ParseCursor(q CatalogQuery) ([]CatalogValue, error) {
	if q.LastID == "" {
		return nil, nil
	}
	if n, err := strconv.ParseInt(q.LastID, 10, 64); err == nil {
		if n < 1 || n > math.MaxInt32 || len(q.Sort) != 1 || q.Sort[0].Field != c.IdentityField() || strconv.FormatInt(n, 10) != q.LastID {
			return nil, common.ErrBadRequest
		}
		value := q.LastID
		return []CatalogValue{{Kind: "integer", Value: &value}}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(q.LastID)
	if err != nil || base64.RawURLEncoding.EncodeToString(b) != q.LastID {
		return nil, common.ErrBadRequest
	}
	var cursor CatalogCursor
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&cursor) != nil || d.Decode(new(any)) != io.EOF || cursor.Version != 1 || cursor.Method != c || cursor.Query != c.QueryHash(q) || len(cursor.Values) != len(q.Sort) {
		return nil, common.ErrBadRequest
	}
	canonical, _ := json.Marshal(cursor)
	if !bytes.Equal(canonical, b) {
		return nil, common.ErrBadRequest
	}
	for i, v := range cursor.Values {
		f, ok := c.Field(q.Sort[i].Field)
		if !ok || v.Kind != f.Kind {
			return nil, common.ErrBadRequest
		}
		if v.Value == nil {
			if !f.Nullable {
				return nil, common.ErrBadRequest
			}
			continue
		}
		text := *v.Value
		switch f.Kind {
		case "integer":
			n, err := strconv.ParseInt(text, 10, 64)
			if err != nil || n < 0 || n == 0 && f.Name != "affected_user_count" || strconv.FormatInt(n, 10) != text {
				return nil, common.ErrBadRequest
			}
			if (f.Name == "id" || f.Name == "application_id" || f.Name == "role_code" || f.Name == "role_id" || f.Name == "granted_by" || f.Name == "revoked_by") && n > math.MaxInt32 {
				return nil, common.ErrBadRequest
			}
		case "bool":
			if text != "true" && text != "false" {
				return nil, common.ErrBadRequest
			}
		case "time":
			at, err := time.Parse(time.RFC3339Nano, text)
			if err != nil || at.IsZero() || at.UTC().Format(time.RFC3339Nano) != text {
				return nil, common.ErrBadRequest
			}
		case "text":
			if !userText(text, 0, len([]rune(text))) {
				return nil, common.ErrBadRequest
			}
		default:
			return nil, common.ErrBadRequest
		}
	}
	return cursor.Values, nil
}

func (c Catalog) NextCursor(q CatalogQuery, values []CatalogValue) string {
	b, _ := json.Marshal(CatalogCursor{Version: 1, Method: c, Query: c.QueryHash(q), Values: values})
	return base64.RawURLEncoding.EncodeToString(b)
}

func catalogText(kind, text string) CatalogValue { return CatalogValue{kind, &text} }
func catalogTime(at *time.Time) CatalogValue {
	if at == nil {
		return CatalogValue{Kind: "time"}
	}
	return catalogText("time", at.UTC().Format(time.RFC3339Nano))
}

func (r RoleSummary) CatalogValue(field string) CatalogValue {
	switch field {
	case "id":
		return catalogText("integer", strconv.FormatUint(r.ID, 10))
	case "code":
		return catalogText("text", r.Code)
	case "name":
		return catalogText("text", r.Name)
	case "description":
		if r.Description == nil {
			return CatalogValue{Kind: "text"}
		}
		return catalogText("text", *r.Description)
	case "application_id":
		return catalogText("integer", strconv.FormatUint(r.ApplicationID, 10))
	case "application_code":
		return catalogText("text", string(r.ApplicationCode))
	case "status":
		return catalogText("text", string(r.Status))
	case "is_system":
		return catalogText("bool", strconv.FormatBool(r.IsSystem))
	case "version":
		return catalogText("integer", strconv.FormatInt(r.Version, 10))
	case "role_code":
		return catalogText("integer", strconv.FormatInt(int64(r.NumericCode), 10))
	case "created_at":
		return catalogTime(r.CreatedAt)
	case "updated_at":
		return catalogTime(r.UpdatedAt)
	case "affected_user_count":
		return catalogText("integer", strconv.FormatUint(r.AffectedUserCount, 10))
	default:
		return CatalogValue{}
	}
}

func (r PermissionItem) CatalogValue(field string) CatalogValue {
	switch field {
	case "id":
		return catalogText("integer", strconv.FormatUint(r.ID, 10))
	case "application_id":
		return catalogText("integer", strconv.FormatUint(r.ApplicationID, 10))
	case "application_code":
		return catalogText("text", string(r.ApplicationCode))
	case "code":
		return catalogText("text", r.Code)
	case "description":
		return catalogText("text", r.Description)
	default:
		return CatalogValue{}
	}
}

func (r ApplicationItem) CatalogValue(field string) CatalogValue {
	switch field {
	case "id":
		return catalogText("integer", strconv.FormatUint(r.ID, 10))
	case "code":
		return catalogText("text", string(r.Code))
	case "name":
		return catalogText("text", r.Name)
	case "status":
		return catalogText("text", string(r.Status))
	case "is_system":
		return catalogText("bool", strconv.FormatBool(r.IsSystem))
	default:
		return CatalogValue{}
	}
}
