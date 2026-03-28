package sshx

import "time"

const (
	defaultPort           = 22
	defaultConnectTimeout = 10 * time.Second
	defaultCommandTimeout = 30 * time.Second
)

// Default ciphers in preference order: AEAD first, then CTR.
var defaultCiphers = []string{
	"aes256-gcm@openssh.com",
	"aes128-gcm@openssh.com",
	"chacha20-poly1305@openssh.com",
	"aes256-ctr",
	"aes192-ctr",
	"aes128-ctr",
}

// Config holds the parameters for an SSH session.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string

	// Prompt expected from the device (e.g. "<ROUTER-PE01>", "RP/0/RSP0/CPU0:ROUTER#").
	// Required. cmdx uses this to detect when a command has finished.
	Prompt string

	// ConnectTimeout is how long to wait for TCP + SSH handshake.
	// Defaults to 10s if zero.
	ConnectTimeout time.Duration

	// CommandTimeout is how long to wait for the prompt after sending a command.
	// Defaults to 30s if zero.
	CommandTimeout time.Duration

	// Ciphers to offer during SSH negotiation, in preference order.
	// If empty, uses cmdx defaults: GCM > ChaCha20 > CTR.
	Ciphers []string
}

// withDefaults returns a copy of the config with zero values replaced by defaults.
func (c Config) withDefaults() Config {
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = defaultConnectTimeout
	}
	if c.CommandTimeout == 0 {
		c.CommandTimeout = defaultCommandTimeout
	}
	if len(c.Ciphers) == 0 {
		c.Ciphers = defaultCiphers
	}
	return c
}

// validate checks that all required fields are set.
func (c Config) validate() error {
	if c.Host == "" {
		return &ConfigError{Field: "Host", Reason: "must not be empty"}
	}
	if c.Username == "" {
		return &ConfigError{Field: "Username", Reason: "must not be empty"}
	}
	if c.Password == "" {
		return &ConfigError{Field: "Password", Reason: "must not be empty"}
	}
	if c.Prompt == "" {
		return &ConfigError{Field: "Prompt", Reason: "must not be empty"}
	}
	return nil
}
