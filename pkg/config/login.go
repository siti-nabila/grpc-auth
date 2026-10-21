package config

// Login security is prepared independently of the coordinated server cutover.
// There are deliberately no key or secret defaults.
type AccessIssuerConfig struct {
	PrivateKeyFile string
	PublicKeyFile  string
	KeyVersion     string
}

type AuthLimiterConfig struct {
	AccountHMACSecret string
}
