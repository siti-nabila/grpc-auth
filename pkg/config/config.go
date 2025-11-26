package config

import (
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/database"
)

type (
	AppConfig struct {
		ApplicationName  string
		Environment      string
		DebugMode        bool
		Port             int
		Host             string
		Timeout          time.Duration
		KeepAlive        time.Duration
		KeepAliveTimeout time.Duration
		KeepAliveIdle    time.Duration

		Database map[string]database.DBConfig
		Services map[string]ServiceConfig
		JWT      JWTConfig
		Logger   LoggerConfig
	}

	ServiceConfig struct {
		Host             string
		Port             int
		KeepAlive        time.Duration
		KeepAliveTimeout time.Duration
	}
	JWTConfig struct {
		SecretKey string
	}
	LoggerConfig struct {
		HTTPMode string
		DBMode   string
	}
)

var (
	appCfg *AppConfig
)

func SetAppConfig(cfg *AppConfig) {
	appCfg = cfg
}

func GetAppConfig() *AppConfig {
	return appCfg
}
