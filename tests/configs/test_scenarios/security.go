package test_scenarios

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/configs"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "limiter worker is canceled and joined on shutdown and listen failure", Run: limiterWorkerLifecycle},
		{Name: "incomplete session pipeline cannot create a server", Run: rejectIncompletePipeline},
		{Name: "six RPCs authenticate caller before user and preserve locale", Run: secureRPCBoundary},
		{Name: "caller full-method denial precedes bearer authentication", Run: callerMethodDenied},
		{Name: "plaintext wrong CA and hostname never reach business", Run: rejectUntrustedTLS},
		{Name: "TLS and caller config failure creates no server", Run: rejectServerConfiguration},
		{Name: "constructor and listener failures clean up server resources", Run: serverFailureCleanup},
		{Name: "typed security config loads with no runtime side effects", Run: loadSecurityConfig},
		{Name: "missing invalid ambiguous or expired caller config fails closed", Run: invalidSecurityConfig},
	}
}

func loadSecurityConfig(t *testing.T) {
	text := fmt.Sprintf(`ApplicationName: auth-test
Timeout: 5s
TLS:
  CertFile: /fixture/cert.pem
  KeyFile: /fixture/key.pem
ServiceAuth:
  Callers:
    - ServiceID: rest-orc
      Current:
        SHA256: %q
        NotAfter: "2035-01-01T00:00:00Z"
      Previous:
        SHA256: %q
        NotAfter: "2030-01-01T00:00:00Z"
        OverlapUntil: "2027-01-01T00:00:00Z"
      Methods: ["/user.v1.UserService/Me"]
      Namespaces: [iam, clm]
`, fixtures.ServiceDigest(fixtures.ServiceToken(1)), fixtures.ServiceDigest(fixtures.ServiceToken(2)))
	before := config.GetAppConfig()
	got, err := configs.ReadConfig(strings.NewReader(text), fixtures.ServiceTime)
	if err != nil {
		t.Fatal(err)
	}
	if got.Timeout != 5*time.Second || got.TLS.CertFile != "/fixture/cert.pem" || len(got.ServiceAuth.Callers) != 1 || got.ServiceAuth.Callers[0].Previous.OverlapUntil != "2027-01-01T00:00:00Z" || got.ServiceAuth.Callers[0].Namespaces[1] != "clm" {
		t.Fatal("typed config fields not loaded")
	}
	if config.GetAppConfig() != before {
		t.Fatal("parser changed global config")
	}
	for _, input := range []string{"", "TLS: [", "Port: private-secret-value"} {
		_, err := configs.ReadConfig(strings.NewReader(input), fixtures.ServiceTime)
		if err == nil || strings.Contains(err.Error(), "private-secret-value") {
			t.Fatal("invalid config accepted or leaked")
		}
	}
}

func invalidSecurityConfig(t *testing.T) {
	cases := []struct {
		name   string
		change func(*config.ServiceAuthConfig)
	}{
		{"no callers", func(c *config.ServiceAuthConfig) { c.Callers = nil }},
		{"missing ID", func(c *config.ServiceAuthConfig) { c.Callers[0].ServiceID = "" }},
		{"duplicate ID", func(c *config.ServiceAuthConfig) { c.Callers = append(c.Callers, c.Callers[0]) }},
		{"missing digest", func(c *config.ServiceAuthConfig) { c.Callers[0].Current.SHA256 = "" }},
		{"invalid digest", func(c *config.ServiceAuthConfig) { c.Callers[0].Current.SHA256 = "private-secret-value" }},
		{"missing expiry", func(c *config.ServiceAuthConfig) { c.Callers[0].Current.NotAfter = "" }},
		{"invalid expiry", func(c *config.ServiceAuthConfig) { c.Callers[0].Current.NotAfter = "later" }},
		{"expired", func(c *config.ServiceAuthConfig) {
			c.Callers[0].Current.NotAfter = fixtures.ServiceTime.Format(time.RFC3339)
		}},
		{"duplicate digest across callers", func(c *config.ServiceAuthConfig) {
			other := c.Callers[0]
			other.ServiceID = "other"
			c.Callers = append(c.Callers, other)
		}},
		{"duplicate current previous", func(c *config.ServiceAuthConfig) { c.Callers[0].Previous.SHA256 = c.Callers[0].Current.SHA256 }},
		{"previous no overlap", func(c *config.ServiceAuthConfig) { c.Callers[0].Previous.OverlapUntil = "" }},
		{"previous expired overlap", func(c *config.ServiceAuthConfig) {
			c.Callers[0].Previous.OverlapUntil = fixtures.ServiceTime.Format(time.RFC3339)
		}},
		{"overlap past previous lifetime", func(c *config.ServiceAuthConfig) { c.Callers[0].Previous.OverlapUntil = "2031-01-01T00:00:00Z" }},
		{"overlap past current lifetime", func(c *config.ServiceAuthConfig) { c.Callers[0].Current.NotAfter = "2026-12-01T00:00:00Z" }},
		{"missing previous expiry", func(c *config.ServiceAuthConfig) { c.Callers[0].Previous.NotAfter = "" }},
		{"empty methods", func(c *config.ServiceAuthConfig) { c.Callers[0].Methods = nil }},
		{"method wildcard", func(c *config.ServiceAuthConfig) { c.Callers[0].Methods = []string{"/user.v1.UserService/*"} }},
		{"method prefix", func(c *config.ServiceAuthConfig) { c.Callers[0].Methods = []string{"/user.v1.UserService"} }},
		{"duplicate method", func(c *config.ServiceAuthConfig) {
			c.Callers[0].Methods = append(c.Callers[0].Methods, c.Callers[0].Methods[0])
		}},
		{"empty namespaces", func(c *config.ServiceAuthConfig) { c.Callers[0].Namespaces = nil }},
		{"namespace wildcard", func(c *config.ServiceAuthConfig) { c.Callers[0].Namespaces = []string{"iam.*"} }},
		{"duplicate namespace", func(c *config.ServiceAuthConfig) { c.Callers[0].Namespaces = []string{"iam", "iam"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := fixtures.ServiceConfig()
			tc.change(&cfg)
			err := cfg.Validate(fixtures.ServiceTime)
			if err == nil || strings.Contains(err.Error(), "private-secret-value") {
				t.Fatal("bad config accepted or leaked")
			}
		})
	}
	for _, cfg := range []config.TLSConfig{{}, {CertFile: "cert"}, {KeyFile: "key"}, {CertFile: " ", KeyFile: "key"}} {
		if cfg.Validate() == nil {
			t.Fatal("missing TLS config accepted")
		}
	}
	cfg := fixtures.ServiceConfig()
	cfg.Callers[0].Previous = nil
	if err := cfg.Validate(fixtures.ServiceTime); err != nil {
		t.Fatal(err)
	}
}
