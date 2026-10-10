package configs

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/config"
)

func serverTLS(cfg config.TLSConfig, now time.Time) (*tls.Config, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pair, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, errors.New("cannot load TLS certificate and matching private key")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, errors.New("TLS certificate must be currently valid")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}}, nil
}
