package configs

import (
	"log"

	"google.golang.org/grpc"
)

func InitAllConfigs() {
	appCfg := &AppConfig{}
	err := appCfg.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load app config: %v", err)
		return
	}

	// init database
	for dbName, v := range appCfg.Database {
		src := dbSource(dbName)
		DBAddConnection(src, v)
		DBConnect(src)
	}
	log.Println("✅ Database connections established")

	svr := NewGRPCServer(appCfg, func(s *grpc.Server) {
		RegisterAll(s)
	})

	if err := svr.Start(); err != nil {
		log.Fatalf("failed to start gRPC server: %v", err)
	}

	log.Println("✅ gRPC server started")

	svr.WaitForShutdown()
	svr.GracefulStop()

}
