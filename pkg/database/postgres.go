package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "github.com/godror/godror"
	_ "github.com/lib/pq"
)

const (
	UserDbSource DbSource = "user"
)

type (
	DbSource string
	DBConfig struct {
		User        string
		Password    string
		Host        string
		Port        string
		Name        string
		Driver      string
		ServiceName string
	}
)

var (
	dbNatConnections = make(map[DbSource]*sql.DB, 0)
	dbConfigs        = make(map[DbSource]DBConfig, 0)
	mu               sync.RWMutex
)

func DBConnect(src DbSource) {

	mu.Lock()
	defer mu.Unlock()

	config, exists := dbConfigs[src]
	if !exists {
		panic("DB configuration not found")
	}

	switch strings.ToLower(config.Driver) {
	case "postgres":
		openPostgresConnection(config, src)
	case "oracle":
		openOracleConnection(config, src)
	default:
		panic("Unsupported database driver")
	}

}

func openPostgresConnection(config DBConfig, src DbSource) {
	var (
		dsn string
	)
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
}

func openOracleConnection(config DBConfig, src DbSource) {
	var (
		dsn string
	)
	dsn = fmt.Sprintf(`user="%s" password="%s" connectString="%s:%s/%s"`,
		config.User,
		config.Password,
		config.Host,
		config.Port,
		config.ServiceName,
	)
	db, err := sql.Open("godror", dsn)
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(1 * time.Hour)
	dbNatConnections[src] = db
	fmt.Println("Connected to Oracle database:", src)
}

func DBClose(src DbSource) {
	sqlDB, exists := dbNatConnections[src]
	if !exists {
		return
	}
	err := sqlDB.Close()
	if err != nil {
		fmt.Println("Error closing database connection:", err)
	}
}

func DBAddConnection(src DbSource, cfg DBConfig) {
	mu.Lock()
	defer mu.Unlock()
	dbConfigs[src] = cfg
}

func DBGetNativePool(src DbSource) *sql.DB {
	mu.Lock()
	defer mu.Unlock()

	if db, exists := dbNatConnections[src]; !exists {
		log.Println("error get db: ", src)
		return nil
	} else {
		return db
	}

}

func DBGetConfig(src DbSource) DBConfig {
	mu.RLock()
	defer mu.RUnlock()

	cfg, ok := dbConfigs[src]
	if !ok {
		log.Fatalf("no DB config found with source: %s", src)
	}

	return cfg
}
