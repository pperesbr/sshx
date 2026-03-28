package sshx

import (
	"context"
	"strings"
	"testing"
)

// Run with:
//
//	SSHX_HOST=nokia-olt SSHX_USER=admin SSHX_PASS=secret SSHX_PROMPT="nokia-olt:operator>#" go test -v -run TestNokiaISAM ./...
//
// Skips automatically if SSHX_HOST is not set.
//
// Note: Nokia 7360 changes the prompt when entering "environment" context:
//   Normal:      nokia-olt:operator>#
//   Environment: nokia-olt:operator>environment#
// We handle this by sending all commands in one shot using Run with
// the final prompt, or by using multiple Run calls with prompt changes.

func TestNokiaISAMLive(t *testing.T) {
	cfg := testEnv(t)

	sess, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	// Disable paging — prompt changes to "...>environment#"
	basePrompt := cfg.Prompt
	envPrompt := strings.Replace(basePrompt, ">#", ">environment#", 1)

	// Enter environment context and set no-more.
	// Need to temporarily expect the environment prompt.
	sess.SetPrompt(envPrompt)

	out1, err := sess.Run(ctx, "environment print no-more")
	if err != nil {
		t.Fatalf("environment print no-more failed: %v", err)
	}
	t.Logf("environment print no-more output:\n%s", out1)

	// Exit back to normal context.
	sess.SetPrompt(basePrompt)

	out2, err := sess.Run(ctx, "exit all")
	if err != nil {
		t.Fatalf("exit all failed: %v", err)
	}
	t.Logf("exit all output:\n%s", out2)

	// Show services.
	out3, err := sess.Run(ctx, "show service service-using")
	if err != nil {
		t.Fatalf("show service service-using failed: %v", err)
	}
	if !strings.Contains(out3, "Services") {
		t.Fatalf("expected Services in output: %q", out3)
	}
	t.Logf("show service service-using output:\n%s", out3)
}
