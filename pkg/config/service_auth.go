package config

import (
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
)

type TLSConfig struct {
	CertFile string
	KeyFile  string
}

type ServiceAuthConfig struct {
	Callers []CallerConfig
}

type CallerConfig struct {
	ServiceID  string
	Current    ServiceCredential
	Previous   *PreviousServiceCredential
	Methods    []string
	Namespaces []string
}

type ServiceCredential struct {
	SHA256   string
	NotAfter string
}

type PreviousServiceCredential struct {
	SHA256       string
	NotAfter     string
	OverlapUntil string
}

var (
	serviceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	methodPattern    = regexp.MustCompile(`^/[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*/[A-Za-z_][A-Za-z0-9_]*$`)
	namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
)

func (c TLSConfig) Validate() error {
	if strings.TrimSpace(c.CertFile) == "" || strings.TrimSpace(c.KeyFile) == "" {
		return errors.New("TLS certificate and private key files are required")
	}
	return nil
}

func (c ServiceAuthConfig) Validate(now time.Time) error {
	if len(c.Callers) == 0 {
		return errors.New("service authentication requires configured callers")
	}
	ids := make(map[string]bool)
	digests := make(map[string]bool)
	for _, caller := range c.Callers {
		if !serviceIDPattern.MatchString(caller.ServiceID) || ids[caller.ServiceID] {
			return errors.New("service caller IDs must be valid and unique")
		}
		ids[caller.ServiceID] = true
		if err := validateAllowlist(caller.Methods, methodPattern); err != nil {
			return errors.New("service caller methods require a unique exact full-method allowlist")
		}
		if err := validateAllowlist(caller.Namespaces, namespacePattern); err != nil {
			return errors.New("service caller namespaces require a unique exact allowlist")
		}
		currentExpiry, err := validateCredential(caller.Current.SHA256, caller.Current.NotAfter, now, digests)
		if err != nil {
			return err
		}
		if caller.Previous != nil {
			if err := validatePrevious(*caller.Previous, currentExpiry, now, digests); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAllowlist(values []string, pattern *regexp.Regexp) error {
	if len(values) == 0 {
		return errors.New("empty allowlist")
	}
	seen := make(map[string]bool)
	for _, value := range values {
		if !pattern.MatchString(value) || seen[value] {
			return errors.New("invalid or duplicate allowlist entry")
		}
		seen[value] = true
	}
	return nil
}

func validateCredential(digest, expiry string, now time.Time, seen map[string]bool) (time.Time, error) {
	value, err := hex.DecodeString(digest)
	if err != nil || len(value) != 32 || seen[strings.ToLower(digest)] {
		return time.Time{}, errors.New("service credential SHA-256 digests must be valid and globally unique")
	}
	until, err := time.Parse(time.RFC3339, expiry)
	if err != nil || !now.Before(until) {
		return time.Time{}, errors.New("service credential not_after must be an explicit future RFC3339 timestamp")
	}
	seen[strings.ToLower(digest)] = true
	return until, nil
}

func validatePrevious(previous PreviousServiceCredential, currentExpiry, now time.Time, digests map[string]bool) error {
	expiry, err := validateCredential(previous.SHA256, previous.NotAfter, now, digests)
	if err != nil {
		return err
	}
	overlap, err := time.Parse(time.RFC3339, previous.OverlapUntil)
	if err != nil || !now.Before(overlap) || overlap.After(expiry) || overlap.After(currentExpiry) {
		return errors.New("previous service credential requires an explicit future overlap_until within both credentials lifetimes")
	}
	return nil
}
