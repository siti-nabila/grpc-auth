package configs

import (
	"log"

	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/logger"
	"google.golang.org/grpc"
)

func InitAllConfigs() {
	appCfg := &AppConfig{}
	err := appCfg.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load app config: %v", err)
		return
	}
	// ===== Init Logger =====
	if err := logger.InitLog(
		logger.TextMode,
		// logger.TextMode,
	); err != nil {
		log.Fatalf("failed to init logger: %v", err)
		return
	}
	logger.Logs.HTTP.Info("✅ HTTP Logger initialized")
	logger.Logs.DB.Info("✅ DB Logger initialized")

	// init database
	for dbName, v := range appCfg.Database {
		src := database.DbSource(dbName)
		database.DBAddConnection(src, v)
		database.DBConnect(src)
		logger.Logs.DB.WithField("database name", dbName).Info("✅ DB connection established")
	}

	svr := NewGRPCServer(appCfg, func(s *grpc.Server) {
		RegisterAll(s)
	})

	if err := svr.Start(); err != nil {
		logger.Logs.HTTP.Fatalf("❌ failed to start gRPC server: %v", err)

	}

	logger.Logs.HTTP.Info("✅ gRPC server started")

	svr.WaitForShutdown()
	svr.GracefulStop()

}
