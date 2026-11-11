package sshx

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// slowReader delivers data in chunks of a given size, simulating
// how SSH sends bytes in variable-sized pieces over the network.
type slowReader struct {
	data      []byte
	chunkSize int
	offset    int
}

func (s *slowReader) Read(p []byte) (int, error) {
	if s.offset >= len(s.data) {
		return 0, io.EOF
	}
	end := s.offset + s.chunkSize
	if end > len(s.data) {
		end = len(s.data)
	}
	n := copy(p, s.data[s.offset:end])
	s.offset += n
	return n, nil
}

// hangReader blocks forever on Read, simulating a silent SSH session.
type hangReader struct{}

func (h *hangReader) Read(p []byte) (int, error) {
	select {} // blocks forever
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSimplePrompt(t *testing.T) {
	input := "interface GigabitEthernet0/0/0\r\n ip address 10.0.0.1\r\n<ROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptWithCiscoHash(t *testing.T) {
	input := "BGP router identifier 10.0.0.1\r\nRP/0/RSP0/CPU0:ROUTER-PE01#"
	prompt := "RP/0/RSP0/CPU0:ROUTER-PE01#"
	r := NewReader(bytes.NewReader([]byte(input)), prompt)

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptWithANSIBeforePrompt(t *testing.T) {
	// \x1b[42D = cursor back 42 positions (common on Huawei VRP)
	input := "some output\r\n\x1b[42D<ROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptWithANSIColorCodes(t *testing.T) {
	// \x1b[0;32m = green color, \x1b[0m = reset
	input := "\x1b[0;32minterface Loopback0\x1b[0m\r\n<ROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptWithANSIInsidePrompt(t *testing.T) {
	// ANSI codes injected between each character of the prompt
	// Simulates: <\x1b[0mROUTER-PE01>
	input := "output\r\n<\x1b[0mROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptWithOSCSequence(t *testing.T) {
	// OSC sets window title, common when SSH connects to some devices
	input := "\x1b]0;ROUTER-PE01\x07show version\r\n<ROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptWithTwoByteEscape(t *testing.T) {
	// \x1bM = reverse index (cursor up), 2-byte escape
	input := "\x1bMsome output\r\n<ROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptSplitAcrossChunks(t *testing.T) {
	// Prompt "<ROUTER-PE01>" arrives in 1-byte chunks,
	// simulating worst-case network fragmentation.
	input := "config output here\r\n<ROUTER-PE01>"
	r := NewReader(&slowReader{
		data:      []byte(input),
		chunkSize: 1, // one byte at a time
	}, "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptSplitInMiddle(t *testing.T) {
	// Prompt arrives split: first chunk has "<ROUTER", second has "-PE01>"
	// Chunk size 25 on "config here\r\n<ROUTER-PE01>" splits the prompt
	input := "config here\r\n<ROUTER-PE01>"
	r := NewReader(&slowReader{
		data:      []byte(input),
		chunkSize: 20, // "<ROUTER" in first chunk, "-PE01>" in second
	}, "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestPromptAppearsInConfigButRealPromptAtEnd(t *testing.T) {
	// The prompt string appears inside a banner/config line AND at the end.
	// The reader should match the first occurrence. This is expected behavior:
	// if the exact prompt string appears in config output, it will match early.
	// This is a known trade-off documented in the design.
	input := "header login \"Welcome to <ROUTER-PE01>\"\r\n#\r\ninterface Gi0/0/0\r\n<ROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Will match at first occurrence of <ROUTER-PE01> inside the banner.
	// Output will be truncated at that point.
	if !bytes.Contains([]byte(output), []byte("<ROUTER-PE01>")) {
		t.Fatalf("output should contain prompt, got: %q", output)
	}
}

func TestTimeoutWhenNoPrompt(t *testing.T) {
	// Reader that blocks forever — simulates router not responding
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r := NewReader(&hangReader{}, "<ROUTER-PE01>")

	_, err := r.WaitForPrompt(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestEOFBeforePrompt(t *testing.T) {
	// SSH session closes before prompt appears
	input := "partial output without prompt"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	_, err := r.WaitForPrompt(ctx(t))
	if err == nil {
		t.Fatal("expected error on EOF, got nil")
	}
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	r := NewReader(&hangReader{}, "<ROUTER-PE01>")

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := r.WaitForPrompt(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestReset(t *testing.T) {
	// One stream, two commands: the second prompt is different
	// (entered config mode). Both outputs come from the same source,
	// as they do on a real session.
	input1 := "output1\r\n<ROUTER-PE01>"
	input2 := "config output\r\n[~ROUTER-PE01]"
	r := NewReader(bytes.NewReader([]byte(input1+input2)), "<ROUTER-PE01>")

	output1, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("first command: unexpected error: %v", err)
	}
	if output1 != input1 {
		t.Fatalf("first command: output mismatch\ngot:  %q\nwant: %q", output1, input1)
	}

	r.Reset("[~ROUTER-PE01]")

	output2, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("second command: unexpected error: %v", err)
	}
	if output2 != input2 {
		t.Fatalf("second command: output mismatch\ngot:  %q\nwant: %q", output2, input2)
	}
}

func TestResetKeepsBytesAfterPromptAcrossChunks(t *testing.T) {
	// Bytes that arrive in the same chunk right after the prompt must not
	// be lost: they belong to the next command.
	input1 := "a\r\n<PE01>"
	input2 := "b\r\n<PE01>"
	r := NewReader(&slowReader{data: []byte(input1 + input2), chunkSize: 7}, "<PE01>")

	out1, err := r.WaitForPrompt(ctx(t))
	if err != nil || out1 != input1 {
		t.Fatalf("first: got %q, %v", out1, err)
	}
	r.Reset("<PE01>")
	out2, err := r.WaitForPrompt(ctx(t))
	if err != nil || out2 != input2 {
		t.Fatalf("second: got %q, %v", out2, err)
	}
}

// pipeSource lets a test write to the Reader source at any time.
func pipeSource(t *testing.T) (*io.PipeReader, *io.PipeWriter) {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	return pr, pw
}

func TestTimeoutDoesNotLoseOutput(t *testing.T) {
	// A WaitForPrompt that times out must not leave a goroutine stealing
	// bytes: output written afterwards reaches the next WaitForPrompt whole.
	pr, pw := pipeSource(t)
	r := NewReader(pr, "<PE01>")

	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := r.WaitForPrompt(short); err == nil {
		t.Fatal("expected timeout")
	}

	go pw.Write([]byte("late output\r\n<PE01>"))

	r.Reset("<PE01>")
	out, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "late output\r\n<PE01>" {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestStopAtCustomPromptThenDrain(t *testing.T) {
	// Collector pattern: stop at a line inside the output (checkpoint),
	// then drain until the real prompt. The next command must start clean.
	stream := "rec 3\r\nrec 2\r\nCHECKPOINT\r\nrec 1\r\nrec 0\r\n<PE01>" +
		"next command output\r\n<PE01>"
	r := NewReader(&slowReader{data: []byte(stream), chunkSize: 5}, "CHECKPOINT")

	out, err := r.WaitForPrompt(ctx(t))
	if err != nil || out != "rec 3\r\nrec 2\r\nCHECKPOINT" {
		t.Fatalf("checkpoint: got %q, %v", out, err)
	}

	r.Reset("<PE01>")
	if _, err := r.WaitForPrompt(ctx(t)); err != nil {
		t.Fatalf("drain: %v", err)
	}

	r.Reset("<PE01>")
	out, err = r.WaitForPrompt(ctx(t))
	if err != nil || out != "next command output\r\n<PE01>" {
		t.Fatalf("next command: got %q, %v", out, err)
	}
}

func TestEOFIsSticky(t *testing.T) {
	r := NewReader(bytes.NewReader([]byte("no prompt")), "<PE01>")
	if _, err := r.WaitForPrompt(ctx(t)); err != io.EOF {
		t.Fatalf("first: expected io.EOF, got %v", err)
	}
	r.Reset("<PE01>")
	if _, err := r.WaitForPrompt(ctx(t)); err != io.EOF {
		t.Fatalf("second: expected io.EOF, got %v", err)
	}
}

func TestNokiaSROSPrompt(t *testing.T) {
	input := "BGP Summary\r\nA:ROUTER-PE01#"
	r := NewReader(bytes.NewReader([]byte(input)), "A:ROUTER-PE01#")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestHuaweiConfigPrompt(t *testing.T) {
	input := "sysname ROUTER-PE01\r\n[~ROUTER-PE01]"
	r := NewReader(bytes.NewReader([]byte(input)), "[~ROUTER-PE01]")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestMultipleANSISequencesMixed(t *testing.T) {
	// Heavy ANSI: color + cursor move + reset + OSC + prompt
	input := "\x1b[0;32m\x1b[42D\x1b[0m\x1b]0;title\x07output\r\n<PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestLargeOutput(t *testing.T) {
	// Simulates a big "display current-configuration" with lots of output
	// before the prompt appears.
	var buf bytes.Buffer
	for i := 0; i < 10000; i++ {
		buf.WriteString("interface GigabitEthernet0/0/" + string(rune('0'+i%10)) + "\r\n")
		buf.WriteString(" ip address 10.0." + string(rune('0'+i%10)) + ".1 255.255.255.252\r\n")
		buf.WriteString("#\r\n")
	}
	buf.WriteString("<ROUTER-PE01>")

	input := buf.String()
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output length mismatch: got %d, want %d", len(output), len(input))
	}
}

func TestEmptyPromptPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic with empty prompt, got none")
		}
	}()
	NewReader(bytes.NewReader([]byte("output")), "")
}

func TestPromptOnlyInput(t *testing.T) {
	// The entire input IS the prompt, nothing else
	input := "<ROUTER-PE01>"
	r := NewReader(bytes.NewReader([]byte(input)), "<ROUTER-PE01>")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}

func TestSingleCharPrompt(t *testing.T) {
	input := "output here\r\n>"
	r := NewReader(bytes.NewReader([]byte(input)), ">")

	output, err := r.WaitForPrompt(ctx(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", output, input)
	}
}
