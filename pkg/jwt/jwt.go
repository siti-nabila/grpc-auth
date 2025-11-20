package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type (
	JwtClaims struct {
		UserId uint64 `json:"user_id"`
		jwt.RegisteredClaims
	}
	JwtRequest struct {
		UserId uint64 `json:"user_id"`
		Issuer string `json:"iss"`
		Secret string `json:"secret_key"`
	}
)

func GenerateJWTToken(req JwtRequest) (string, error) {

	claims := JwtClaims{
		UserId: req.UserId,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    req.Issuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(req.Secret))

}
