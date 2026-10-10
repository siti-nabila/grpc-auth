package fixtures

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/pkg/config"
)

var ServiceTime = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

func ServiceToken(value byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

func ServiceDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func ServiceConfig() config.ServiceAuthConfig {
	return config.ServiceAuthConfig{Callers: []config.CallerConfig{{
		ServiceID: "rest-orc",
		Current:   config.ServiceCredential{SHA256: ServiceDigest(ServiceToken(1)), NotAfter: "2035-01-01T00:00:00Z"},
		Previous:  &config.PreviousServiceCredential{SHA256: ServiceDigest(ServiceToken(2)), NotAfter: "2030-01-01T00:00:00Z", OverlapUntil: "2027-01-01T00:00:00Z"},
		Methods: []string{userv1.UserService_Register_FullMethodName, userv1.UserService_Login_FullMethodName,
			userv1.UserService_TesRPC_FullMethodName, userv1.UserService_Me_FullMethodName,
			userv1.UserService_ListUsers_FullMethodName, profilev1.ProfileService_UpdateProfile_FullMethodName},
		Namespaces: []string{"iam", "clm"},
	}}}
}

// ServiceCertificate uses a deterministic test-only key and validity interval.
// No deployment material or runtime entropy is read to construct this fixture.
func ServiceCertificate(t *testing.T, seed byte, notBefore, notAfter time.Time) (config.TLSConfig, []byte) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	template := &x509.Certificate{
		SerialNumber: big.NewInt(int64(seed)), Subject: pkix.Name{CommonName: "auth.test"}, DNSNames: []string{"auth.test"},
		NotBefore: notBefore, NotAfter: notAfter, BasicConstraintsValid: true, IsCA: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(bytes.NewReader(bytes.Repeat([]byte{seed}, 1024)), template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
	dir := t.TempDir()
	files := config.TLSConfig{CertFile: filepath.Join(dir, "cert.pem"), KeyFile: filepath.Join(dir, "key.pem")}
	if err := os.WriteFile(files.CertFile, cert, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.KeyFile, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return files, cert
}
