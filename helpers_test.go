package sshx

import (
	"os"
	"testing"
	"time"
)

func testEnv(t *testing.T) Config {
	t.Helper()

	host := os.Getenv("SSHX_HOST")
	if host == "" {
		t.Skip("SSHX_HOST not set, skipping live test")
	}

	user := os.Getenv("SSHX_USER")
	if user == "" {
		t.Fatal("SSHX_USER is required")
	}

	pass := os.Getenv("SSHX_PASS")
	if pass == "" {
		t.Fatal("SSHX_PASS is required")
	}

	prompt := os.Getenv("SSHX_PROMPT")
	if prompt == "" {
		t.Fatal("SSHX_PROMPT is required")
	}

	return Config{
		Host:           host,
		Username:       user,
		Password:       pass,
		Prompt:         prompt,
		ConnectTimeout: 10 * time.Second,
		CommandTimeout: 30 * time.Second,
	}
}
