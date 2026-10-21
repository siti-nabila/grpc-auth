package serviceauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

const MetadataKey = "x-service-token"

type credential struct {
	digest   []byte
	notAfter time.Time
}

type configuredCaller struct {
	caller      Caller
	methods     map[string]bool
	credentials []credential
}

type Verifier struct {
	callers []configuredCaller
	now     func() time.Time
}

func NewVerifier(cfg config.ServiceAuthConfig, now func() time.Time) (*Verifier, error) {
	if now == nil {
		return nil, errors.New("service verifier requires a clock")
	}
	if err := cfg.Validate(now()); err != nil {
		return nil, err
	}
	v := &Verifier{now: now}
	for _, value := range cfg.Callers {
		entry := configuredCaller{
			caller:      Caller{id: value.ServiceID, namespaces: make(map[string]bool)},
			methods:     make(map[string]bool),
			credentials: []credential{parseCredential(value.Current.SHA256, value.Current.NotAfter)},
		}
		for _, method := range value.Methods {
			entry.methods[method] = true
		}
		for _, namespace := range value.Namespaces {
			entry.caller.namespaces[namespace] = true
		}
		if value.Previous != nil {
			previous := parseCredential(value.Previous.SHA256, value.Previous.NotAfter)
			overlap, _ := time.Parse(time.RFC3339, value.Previous.OverlapUntil)
			if overlap.Before(previous.notAfter) {
				previous.notAfter = overlap
			}
			entry.credentials = append(entry.credentials, previous)
		}
		v.callers = append(v.callers, entry)
	}
	return v, nil
}

func parseCredential(digest, expiry string) credential {
	decoded, _ := hex.DecodeString(digest) // Validated once at construction.
	until, _ := time.Parse(time.RFC3339, expiry)
	return credential{digest: decoded, notAfter: until}
}

// Authenticate requires TLS and exactly one canonical base64url opaque token.
// Neither browser identity metadata nor the user's bearer token selects a caller.
func (v *Verifier) Authenticate(ctx context.Context, fullMethod string) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return ctx, err
	}
	if !securePeer(ctx) {
		return ctx, auth.ErrServiceAuthenticationFailed
	}
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(MetadataKey)
	if len(values) != 1 {
		return ctx, auth.ErrServiceAuthenticationFailed
	}
	if len(values[0]) < 43 || len(values[0]) > 342 {
		return ctx, auth.ErrServiceAuthenticationFailed
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(values[0])
	if err != nil || len(raw) < 32 || len(raw) > 256 || base64.RawURLEncoding.EncodeToString(raw) != values[0] {
		return ctx, auth.ErrServiceAuthenticationFailed
	}
	digest := sha256.Sum256([]byte(values[0]))
	matched := -1
	now := v.now()
	for i, entry := range v.callers {
		for _, key := range entry.credentials {
			equal := subtle.ConstantTimeCompare(digest[:], key.digest)
			if equal == 1 && now.Before(key.notAfter) {
				matched = i
			}
		}
	}
	if matched < 0 {
		return ctx, auth.ErrServiceAuthenticationFailed
	}
	entry := v.callers[matched]
	if !entry.methods[fullMethod] {
		return ctx, common.ErrForbidden
	}
	return context.WithValue(ctx, callerKey{}, entry.caller), nil
}

func securePeer(ctx context.Context) bool {
	value, ok := peer.FromContext(ctx)
	if !ok {
		return false
	}
	info, ok := value.AuthInfo.(credentials.TLSInfo)
	return ok && info.State.HandshakeComplete
}
