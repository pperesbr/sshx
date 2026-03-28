package sshx

import "fmt"

// ConfigError indicates a missing or invalid configuration field.
type ConfigError struct {
	Field  string
	Reason string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("sshx: config %s: %s", e.Field, e.Reason)
}
