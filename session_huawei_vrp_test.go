package sshx

import (
	"context"
	"strings"
	"testing"
)

func TestHuaweiVRPLive(t *testing.T) {
	cfg := testEnv(t)

	sess, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	// Disable paging.
	out1, err := sess.Run(ctx, "screen-length 0 temporary")
	if err != nil {
		t.Fatalf("screen-length failed: %v", err)
	}
	t.Logf("screen-length output:\n%s", out1)

	// Show BGP peers.
	out2, err := sess.Run(ctx, "display bgp peer")
	if err != nil {
		t.Fatalf("display bgp peer failed: %v", err)
	}
	if !strings.Contains(out2, "Established") {
		t.Logf("WARNING: no Established peers found")
	}
	t.Logf("display bgp peer output:\n%s", out2)
}
