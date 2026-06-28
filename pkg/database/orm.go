package database

import (
	"strings"

	orm "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
)

func GetORMConfig() orm.Config {
	// karena pakai postgres sekarang, tidak perlu quote identifier untuk query building nya, dan juga placeholder mode nya menggunakan number

	return orm.Config{
		UseSnakeCase:    true,
		QuoteIdentifier: false,
		PlaceholderMode: orm.PlaceholderByNumber,
		EnableDebug:     true,
	}
}

func GetDialect(src DbSource) dialect.Dialector {
	cfg := DBGetConfig(src)

	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "postgres":
		return dialect.NewPostgres()
	case "oracle":
		return dialect.NewOracle()
	case "mysql":
		return dialect.NewMysql()
	default:
		panic("unsupported dialect")
	}
}
