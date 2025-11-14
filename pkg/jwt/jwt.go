package jwt

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/siti-nabila/grpc-auth/pkg/configs"
)

func GenerateJWTToken(userId int, secretKey string) (string, error) {

	claims := jwt.MapClaims{
		"user_id": strconv.Itoa(userId),
		"exp":     time.Now().Add(time.Hour * 72).Unix(),
		"iat":     time.Now().Unix(),
		"iss":     configs.AppCfg.ApplicationName,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))

}
