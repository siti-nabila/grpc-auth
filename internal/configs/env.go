package configs

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/config"
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

		Database           map[string]database.DBConfig
		Services           map[string]ServiceConfig
		JWT                JWTConfig
		Logger             LoggerConfig
		TLS                config.TLSConfig
		ServiceAuth        config.ServiceAuthConfig
		AccessIssuer       config.AccessIssuerConfig
		AuthLimiter        config.AuthLimiterConfig
		CommandFingerprint config.CommandFingerprintConfig
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

// var AppCfg *AppConfig

func (c *AppConfig) LoadConfig() error {
	f, err := os.Open("env.yaml")
	if err != nil {
		return fmt.Errorf("open env configuration: %w", err)
	}
	defer f.Close()
	loaded, err := ReadConfig(f, time.Now())
	if err != nil {
		return err
	}
	*c = *loaded
	c.SetConfigPackage()
	log.Println("Config loaded from env.yaml")
	return nil
}

// ReadConfig parses and validates configuration without publishing global state
// or opening logger/database/network resources.
func ReadConfig(reader io.Reader, now time.Time) (*AppConfig, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(reader); err != nil {
		return nil, errors.New("invalid env configuration YAML")
	}
	c := new(AppConfig)
	if err := v.Unmarshal(c); err != nil {
		return nil, errors.New("invalid env configuration types")
	}
	if err := c.TLS.Validate(); err != nil {
		return nil, err
	}
	if err := c.ServiceAuth.Validate(now); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *AppConfig) SetConfigPackage() {
	cfgSvcs := make(map[string]config.ServiceConfig, 0)
	for svcName, v := range c.Services {
		cfgSvcs[svcName] = config.ServiceConfig{
			Host:             v.Host,
			Port:             v.Port,
			KeepAlive:        v.KeepAlive,
			KeepAliveTimeout: v.KeepAliveTimeout,
		}
	}

	config.SetAppConfig(&config.AppConfig{
		ApplicationName:    c.ApplicationName,
		Environment:        c.Environment,
		DebugMode:          c.DebugMode,
		Port:               c.Port,
		Host:               c.Host,
		Timeout:            c.Timeout,
		KeepAlive:          c.KeepAlive,
		KeepAliveTimeout:   c.KeepAliveTimeout,
		KeepAliveIdle:      c.KeepAliveIdle,
		Database:           c.Database,
		AccessIssuer:       c.AccessIssuer,
		AuthLimiter:        c.AuthLimiter,
		CommandFingerprint: c.CommandFingerprint,
		Services:           cfgSvcs,
		JWT: config.JWTConfig{
			SecretKey: c.JWT.SecretKey,
		},
		Logger: config.LoggerConfig{
			HTTPMode: c.Logger.HTTPMode,
			DBMode:   c.Logger.DBMode,
		},
	})
}

// func GetAppConfig() *AppConfig {
// 	return AppCfg
// }
