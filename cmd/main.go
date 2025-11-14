package main

import (
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/configs"
)

func init() {
	configs.InitAllConfigs()
}
func main() {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	time.Local = loc

}
