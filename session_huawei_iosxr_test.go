package sshx

import (
	"context"
	"strings"
	"testing"
)

// Run with:
//
//	SSHX_HOST=10.0.0.1 SSHX_USER=admin SSHX_PASS=secret SSHX_PROMPT="RP/0/RSP0/CPU0:ROUTER#" go test -v -run TestCiscoIOSXR ./...
//
// Skips automatically if SSHX_HOST is not set.

func TestCiscoIOSXRLive(t *testing.T) {
	cfg := testEnv(t)

	sess, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	// Disable paging.
	out1, err := sess.Run(ctx, "terminal length 0")
	if err != nil {
		t.Fatalf("terminal length failed: %v", err)
	}
	t.Logf("terminal length output:\n%s", out1)

	// Show BGP summary.
	out2, err := sess.Run(ctx, "show bgp summary")
	if err != nil {
		t.Fatalf("show bgp summary failed: %v", err)
	}

	if !strings.Contains(out2, "BGP router identifier") {
		t.Fatalf("expected BGP router identifier in output: %q", out2)
	}
	t.Logf("show bgp summary output:\n%s", out2)
}
