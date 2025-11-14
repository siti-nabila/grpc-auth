package configs

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

const (
	UserDbSource dbSource = "user"
)

type (
	dbSource string
	DBConfig struct {
		User     string
		Password string
		Host     string
		Port     string
		Name     string
		Driver   string
	}
)

var (
	dbNatConnections = make(map[dbSource]*sql.DB, 0)
	dbConfigs        = make(map[dbSource]DBConfig, 0)
	mu               sync.RWMutex
)

func DBConnect(src dbSource) {

	mu.Lock()
	defer mu.Unlock()
	var (
		dsn string
	)

	config, exists := dbConfigs[src]
	if !exists {
		panic("DB configuration not found")
	}

	switch strings.ToLower(config.Driver) {
	case "postgres":
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable timezone=Asia/Jakarta",
			config.Host,
			config.Port,
			config.User,
			config.Password,
			config.Name,
		)
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			panic(err)
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(25)
		db.SetConnMaxLifetime(1 * time.Hour)
		dbNatConnections[src] = db
		fmt.Println("Connected to Postgres database:", src)
	default:
		panic("Unsupported database driver")
	}

}

func DBClose(src dbSource) {
	sqlDB, exists := dbNatConnections[src]
	if !exists {
		return
	}
	err := sqlDB.Close()
	if err != nil {
		fmt.Println("Error closing database connection:", err)
	}
}

func DBAddConnection(src dbSource, cfg DBConfig) {
	mu.Lock()
	defer mu.Unlock()
	dbConfigs[src] = cfg
}
