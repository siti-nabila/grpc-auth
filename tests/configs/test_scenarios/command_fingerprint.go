package test_scenarios

import (
	"errors"
	"strings"
	"testing"
	"time"

	errorscommon "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/configs"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func CommandFingerprint() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "command fingerprint rejects missing weak unknown and shared secrets", Run: commandFingerprintValidation},
		{Name: "retained configured keys replay after active rotation and missing keys fail closed", Run: commandFingerprintRotation},
	}
}

func commandFingerprintConfig() *configs.AppConfig {
	c := &configs.AppConfig{CommandFingerprint: config.CommandFingerprintConfig{ActiveVersion: "v1", Keys: map[string]string{"v1": strings.Repeat("a", 32)}}}
	c.JWT.SecretKey = strings.Repeat("j", 32)
	c.AuthLimiter.AccountHMACSecret = strings.Repeat("l", 32)
	return c
}

func commandFingerprintValidation(t *testing.T) {
	for name, change := range map[string]func(*configs.AppConfig){
		"missing active":  func(c *configs.AppConfig) { c.CommandFingerprint.ActiveVersion = "" },
		"unknown active":  func(c *configs.AppConfig) { c.CommandFingerprint.ActiveVersion = "v2" },
		"missing keys":    func(c *configs.AppConfig) { c.CommandFingerprint.Keys = nil },
		"short":           func(c *configs.AppConfig) { c.CommandFingerprint.Keys["v1"] = "short" },
		"invalid version": func(c *configs.AppConfig) { c.CommandFingerprint.Keys["bad:version"] = strings.Repeat("b", 32) },
		"jwt reuse":       func(c *configs.AppConfig) { c.CommandFingerprint.Keys["v1"] = c.JWT.SecretKey },
		"limiter reuse":   func(c *configs.AppConfig) { c.CommandFingerprint.Keys["v1"] = c.AuthLimiter.AccountHMACSecret },
		"duplicate key":   func(c *configs.AppConfig) { c.CommandFingerprint.Keys["v2"] = c.CommandFingerprint.Keys["v1"] },
	} {
		t.Run(name, func(t *testing.T) {
			c := commandFingerprintConfig()
			change(c)
			f, err := configs.LoadCommandFingerprinter(c)
			if err == nil || f != nil || strings.Contains(err.Error(), c.CommandFingerprint.Keys["v1"]) && c.CommandFingerprint.Keys["v1"] != "" {
				t.Fatal("invalid config accepted or secret exposed")
			}
		})
	}
	if f, err := configs.LoadCommandFingerprinter(nil); err == nil || f != nil {
		t.Fatal("nil config accepted")
	}
}

func commandFingerprintRotation(t *testing.T) {
	c := commandFingerprintConfig()
	f, err := configs.LoadCommandFingerprinter(c)
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.IdempotencyScope{ActorID: 42, Method: "/user.v1.UserService/CreateUser", Target: "account/ana@example.test", Key: "creation-1"}
	payload := common.FingerprintPayload{Email: "ana@example.test", Status: "ACTIVE"}
	password := []byte("synthetic-password")
	hash, err := f.PasswordFingerprint(scope, payload, password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	record := &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: `{"user_id":42,"version":1,"changed":true}`, ExpiresAt: now.Add(domain.IdempotencyWindow)}
	c.CommandFingerprint.ActiveVersion = "v2"
	c.CommandFingerprint.Keys["v2"] = strings.Repeat("b", 32)
	rotated, err := configs.LoadCommandFingerprinter(c)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rotated.ReplayPassword(record, scope, payload, password, now)
	if err != nil || result == nil || result.UserID != 42 {
		t.Fatal("retained replay failed", err)
	}
	delete(c.CommandFingerprint.Keys, "v1")
	removed, err := configs.LoadCommandFingerprinter(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = removed.ReplayPassword(record, scope, payload, password, now); !errors.Is(err, errorscommon.ErrDependencyUnavailable) {
		t.Fatal("missing retained key did not fail closed", err)
	}
	if result, err = removed.ReplayPassword(record, scope, payload, password, record.ExpiresAt); err != nil || result != nil {
		t.Fatal("expired record still needs retained key", err)
	}
}
