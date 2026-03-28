package sshx

import (
	"context"
	"strings"
	"testing"
)

// Run with:
//
//	SSHX_HOST=huawei-olt SSHX_USER=admin SSHX_PASS=secret SSHX_PROMPT="huawei-olt>" go test -v -run TestHuaweiOLT ./...
//
// Skips automatically if SSHX_HOST is not set.
//
// Note: MA5800 uses "undo smart" to disable interactive prompts
// and "scroll" to disable paging.

func TestHuaweiOLTLive(t *testing.T) {
	cfg := testEnv(t)

	sess, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	// Disable interactive prompts.
	out1, err := sess.Run(ctx, "undo smart")
	if err != nil {
		t.Fatalf("undo smart failed: %v", err)
	}
	t.Logf("undo smart output:\n%s", out1)

	// Disable paging.
	out2, err := sess.Run(ctx, "scroll")
	if err != nil {
		t.Fatalf("scroll failed: %v", err)
	}
	t.Logf("scroll output:\n%s", out2)

	// Show routing table (large output).
	out3, err := sess.Run(ctx, "display ip routing-table")
	if err != nil {
		t.Fatalf("display ip routing-table failed: %v", err)
	}
	if !strings.Contains(out3, "Routing Table") {
		t.Fatalf("expected Routing Table in output: %q", out3)
	}
	t.Logf("display ip routing-table: %d bytes", len(out3))
	t.Logf("display ip routing-table output:\n%s", out3)

}
