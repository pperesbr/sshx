package sshx

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestHuaweiVRPInterruptLive valida Interrupt + WaitPrompt contra uma caixa real:
//  1. inicia um display longo e para numa linha do meio da saída (SetPrompt);
//  2. interrompe com Ctrl+C e drena até o prompt normal;
//  3. confirma que os próximos comandos saem limpos, sem sobra do display
//     nem prompt extra deixado pelo Ctrl+C.
func TestHuaweiVRPInterruptLive(t *testing.T) {
	cfg := testEnv(t)
	cfg.CommandTimeout = 60 * time.Second

	sess, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	if _, err := sess.Run(ctx, "screen-length 0 temporary"); err != nil {
		t.Fatalf("screen-length failed: %v", err)
	}

	// 1. para na primeira linha "sysname" da configuração, bem no começo da saída
	sess.SetPrompt("sysname")
	out, err := sess.Run(ctx, "display current-configuration")
	sess.SetPrompt(cfg.Prompt)
	if err != nil {
		t.Fatalf("display current-configuration failed: %v", err)
	}
	if !strings.HasSuffix(out, "sysname") {
		t.Fatalf("expected output to stop at \"sysname\", got tail: %q", tail(out, 200))
	}
	t.Logf("stopped at custom prompt after %d bytes", len(out))

	// 2. interrompe e drena até o prompt normal
	if err := sess.Interrupt(); err != nil {
		t.Fatalf("interrupt failed: %v", err)
	}
	drained, err := sess.WaitPrompt(ctx)
	if err != nil {
		t.Fatalf("drain failed: %v", err)
	}
	t.Logf("drained %d bytes, tail: %q", len(drained), tail(drained, 200))

	// 3. dois comandos seguidos precisam devolver a própria saída.
	// Se o Ctrl+C tivesse deixado um prompt extra, o primeiro Run voltaria
	// só com esse prompt, sem a data.
	for i := 1; i <= 2; i++ {
		clock, err := sess.Run(ctx, "display clock")
		if err != nil {
			t.Fatalf("display clock #%d failed: %v", i, err)
		}
		if !strings.Contains(clock, "20") || strings.Contains(clock, "interface ") {
			t.Fatalf("display clock #%d came out dirty: %q", i, tail(clock, 300))
		}
		t.Logf("display clock #%d:\n%s", i, clock)
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
