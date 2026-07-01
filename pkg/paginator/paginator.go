package paginator

import (
	"errors"
	"strconv"
	"strings"

	errorpackage "github.com/siti-nabila/error-package"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/pagination"
)

type (
	Mode string

	CursorParser func(string) (any, error)

	CursorConfig struct {
		Field        string
		Value        string
		RequestField string
		Parser       CursorParser
		EmptyValues  []string
	}

	Config struct {
		Mode         Mode
		DefaultLimit int
		MaxLimit     int
		BatchLimit   int
		Cursor       *CursorConfig
		DefaultSort  []orm.SortField
	}

	Result struct {
		Options orm.QueryOptions
		Page    int
		Limit   int
	}
)

const (
	ModeOffset Mode = "offset"
	ModeCursor Mode = "cursor"
)

func Build(opts orm.QueryOptions, cfg Config) (Result, error) {
	cfg = normalizeConfig(cfg)

	page, limit := normalizePageLimit(opts.Page, opts.Limit, cfg)
	opts.Page = page
	opts.Limit = limit

	if len(opts.Sort) == 0 && len(cfg.DefaultSort) > 0 {
		opts.Sort = append([]orm.SortField(nil), cfg.DefaultSort...)
	}

	if cfg.Mode == ModeCursor {
		inMemoryOffset, err := buildInMemoryOffset(cfg)
		if err != nil {
			return Result{}, err
		}
		opts.InMemoryOffset = inMemoryOffset
	} else {
		opts.InMemoryOffset = nil
	}

	return Result{
		Options: opts,
		Page:    page,
		Limit:   limit,
	}, nil
}

func PositiveInt64Cursor(value string) (any, error) {
	value = strings.TrimSpace(value)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return nil, errors.New("cursor must be a positive integer")
	}
	return id, nil
}

func NewValidationError(field, message string) error {
	errs := errorpackage.Errors{}
	errs.Add(field, errors.New(message))
	return errs
}

func normalizeConfig(cfg Config) Config {
	if cfg.Mode == "" {
		cfg.Mode = ModeOffset
	}
	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = pagination.DefaultLimit
	}
	if cfg.MaxLimit <= 0 {
		cfg.MaxLimit = pagination.MaxLimit
	}
	if cfg.DefaultLimit > cfg.MaxLimit {
		cfg.DefaultLimit = cfg.MaxLimit
	}
	if cfg.BatchLimit <= 0 || cfg.BatchLimit > cfg.MaxLimit {
		cfg.BatchLimit = cfg.MaxLimit
	}
	return cfg
}

func normalizePageLimit(page, limit int, cfg Config) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = cfg.DefaultLimit
	}
	if limit > cfg.MaxLimit {
		limit = cfg.MaxLimit
	}
	return page, limit
}

func buildInMemoryOffset(cfg Config) (*orm.InMemoryOffsetOptions, error) {
	if cfg.Cursor == nil {
		return nil, NewValidationError("cursor", "cursor config is required")
	}

	cursorField := strings.TrimSpace(cfg.Cursor.Field)
	if cursorField == "" {
		return nil, NewValidationError(cursorRequestField(*cfg.Cursor), "cursor field is required")
	}

	value, err := parseCursorValue(*cfg.Cursor)
	if err != nil {
		return nil, err
	}

	return &orm.InMemoryOffsetOptions{
		Cursor: orm.Cursor{
			Field: cursorField,
			Value: value,
		},
		MaxLimit: cfg.BatchLimit,
	}, nil
}

func parseCursorValue(cfg CursorConfig) (any, error) {
	value := strings.TrimSpace(cfg.Value)
	if isEmptyCursorValue(value, cfg.EmptyValues) {
		return "", nil
	}
	if cfg.Parser == nil {
		return value, nil
	}

	parsed, err := cfg.Parser(value)
	if err != nil {
		return nil, NewValidationError(cursorRequestField(cfg), err.Error())
	}
	return parsed, nil
}

func isEmptyCursorValue(value string, emptyValues []string) bool {
	if value == "" || value == "0" {
		return true
	}
	for _, empty := range emptyValues {
		if value == strings.TrimSpace(empty) {
			return true
		}
	}
	return false
}

func cursorRequestField(cfg CursorConfig) string {
	field := strings.TrimSpace(cfg.RequestField)
	if field == "" {
		return "cursor"
	}
	return field
}
