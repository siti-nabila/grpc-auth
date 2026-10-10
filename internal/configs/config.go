package configs

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/logger"
	"github.com/siti-nabila/orm/dialect"
)

func InitAllConfigs() {
	appCfg := &AppConfig{}
	err := appCfg.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load app config: %v", err)
		return
	}
	if err := run(appCfg); err != nil {
		log.Fatalf("failed to start gRPC service: %v", err)
	}
}

func run(appCfg *AppConfig) error {
	// Reject security configuration before initializing logger, pools or listeners.
	security, err := LoadLoginSecurity(appCfg, os.ReadFile, time.Now)
	if err != nil {
		return err
	}
	if _, err := serverTLS(appCfg.TLS, time.Now()); err != nil {
		return err
	}
	if err := appCfg.ServiceAuth.Validate(time.Now()); err != nil {
		return err
	}
	primary, ok := appCfg.Database[string(database.UserDbSource)]
	if !ok || strings.ToLower(strings.TrimSpace(primary.Driver)) != "postgres" {
		return fmt.Errorf("session foundation requires the primary user PostgreSQL configuration")
	}
	var opened []database.DbSource
	var svr *GRPCServerClient
	defer func() {
		if svr != nil {
			svr.GracefulStop()
		}
		for i := len(opened) - 1; i >= 0; i-- {
			database.DBClose(opened[i])
		}
	}()
	// ===== Init Logger =====
	if err := logger.InitLog(
		logger.TextMode,
		// logger.TextMode,
	); err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	logger.Logs.HTTP.Info("✅ HTTP Logger initialized")
	logger.Logs.DB.Info("✅ DB Logger initialized")

	// init database
	names := make([]string, 0, len(appCfg.Database))
	for name := range appCfg.Database {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, dbName := range names {
		v := appCfg.Database[dbName]
		src := database.DbSource(dbName)
		database.DBAddConnection(src, v)
		if err := database.DBConnectChecked(src); err != nil {
			return err
		}
		opened = append(opened, src)
		logger.Logs.DB.WithField("database name", dbName).Info("✅ DB connection established")
	}
	foundation, err := PrepareFoundation(context.Background(), security, database.DBGetNativePool(database.UserDbSource), dialect.NewPostgres(), database.GetORMConfig(), time.Now, rand.Reader)
	if err != nil {
		return err
	}
	svr, err = NewGRPCServer(appCfg, foundation.Pipeline)
	if err != nil {
		return err
	}

	if err := svr.Start(); err != nil {
		return err
	}

	logger.Logs.HTTP.Info("✅ gRPC server started")

	svr.WaitForShutdown()
	return nil
}
