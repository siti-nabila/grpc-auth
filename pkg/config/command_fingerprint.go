package config

// CommandFingerprintConfig retains older versions for the full replay window.
// Values are secrets supplied by deployment configuration, never defaults.
type CommandFingerprintConfig struct {
	ActiveVersion string
	Keys          map[string]string
}
