package configs

import (
	"fmt"
	"log"
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/spf13/viper"
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

var AppCfg *AppConfig

func (c *AppConfig) LoadConfig() error {
	v := viper.New()
	v.SetConfigName("env")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("error when reading config env file: %v", err)
	}

	if err := v.Unmarshal(c); err != nil {
		return fmt.Errorf("error when unmarshalling config env file: %v", err)
	}
	AppCfg = c

	log.Println("✅ Config loaded from:", v.ConfigFileUsed())

	return nil

}

func GetAppConfig() *AppConfig {
	return AppCfg
}
