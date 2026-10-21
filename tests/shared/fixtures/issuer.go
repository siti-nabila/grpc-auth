package fixtures

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func AccessPEM(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key := AccessKey()
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})
}
