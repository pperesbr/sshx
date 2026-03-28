package sshx

import (
	"context"
	"strings"
	"testing"
)

// Run with:
//
//	SSHX_HOST=10.0.0.1 SSHX_USER=admin SSHX_PASS=secret SSHX_PROMPT="A:ROUTER#" go test -v -run TestNokiaSROS ./...
//
// Skips automatically if SSHX_HOST is not set.

func TestNokiaSROSLive(t *testing.T) {
	cfg := testEnv(t)

	sess, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	// Disable paging.
	out1, err := sess.Run(ctx, "environment no more")
	if err != nil {
		t.Fatalf("environment no more failed: %v", err)
	}
	t.Logf("environment no more output:\n%s", out1)

	// Show BGP summary.
	out2, err := sess.Run(ctx, "show router bgp summary")
	if err != nil {
		t.Fatalf("show router bgp summary failed: %v", err)
	}

	if !strings.Contains(out2, "BGP Router ID") {
		t.Fatalf("expected BGP Router ID in output: %q", out2)
	}
	t.Logf("show router bgp summary output:\n%s", out2)
}
