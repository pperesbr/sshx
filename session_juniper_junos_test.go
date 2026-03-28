package sshx

import (
	"context"
	"strings"
	"testing"
)

// Run with:
//
//	SSHX_HOST=juniper-router SSHX_USER=admin SSHX_PASS=secret SSHX_PROMPT="operator@juniper-router-re0>" go test -v -run TestJuniperJunos ./...
//
// Skips automatically if SSHX_HOST is not set.
//
// Note: Juniper prompt does NOT include "{master}" prefix.
// The prompt is just "user@HOSTNAME>" (operational mode)
// or "user@HOSTNAME#" (config mode).

func TestJuniperJunosLive(t *testing.T) {
	cfg := testEnv(t)

	sess, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	// Disable paging.
	out1, err := sess.Run(ctx, "set cli screen-length 0")
	if err != nil {
		t.Fatalf("set cli screen-length failed: %v", err)
	}
	if !strings.Contains(out1, "Screen length set to 0") {
		t.Fatalf("unexpected screen-length output: %q", out1)
	}
	t.Logf("set cli screen-length output:\n%s", out1)

	// Show BGP summary.
	out2, err := sess.Run(ctx, "show bgp summary")
	if err != nil {
		t.Fatalf("show bgp summary failed: %v", err)
	}
	if !strings.Contains(out2, "Peers") {
		t.Fatalf("expected Peers in output: %q", out2)
	}
	t.Logf("show bgp summary output:\n%s", out2)
}
