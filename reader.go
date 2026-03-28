package sshx

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// ansiState tracks where we are inside an ANSI escape sequence.
type ansiState int

const (
	stateNormal ansiState = iota
	stateEsc
	stateCSI
	stateOSC
)

// Reader processes bytes from an io.Reader (SSH stdout), strips ANSI
// escape sequences via a finite state machine, and detects a prompt
// using a sliding window implemented as a circular buffer.
type Reader struct {
	src    io.Reader
	prompt []byte

	// Circular buffer — holds the last len(prompt) clean bytes.
	window []byte
	pos    int
	filled int

	// ANSI state machine
	state ansiState

	// Full raw output including ANSI codes
	raw bytes.Buffer
}

// NewReader creates a Reader that reads from src and detects the given prompt.
// Panics if prompt is empty — this is a programming error, not a runtime error.
func NewReader(src io.Reader, prompt string) *Reader {
	if len(prompt) == 0 {
		panic("cmdx: prompt must not be empty")
	}
	p := []byte(prompt)
	return &Reader{
		src:    src,
		prompt: p,
		window: make([]byte, len(p)),
	}
}

// feedByte processes one raw byte through the ANSI state machine.
// Clean bytes (not part of an escape sequence) are pushed into the
// sliding window. Returns true when the prompt is detected.
func (r *Reader) feedByte(b byte) bool {
	r.raw.WriteByte(b)

	switch r.state {
	case stateNormal:
		if b == 0x1b {
			r.state = stateEsc
			return false
		}
		return r.pushClean(b)

	case stateEsc:
		switch b {
		case '[':
			r.state = stateCSI
		case ']':
			r.state = stateOSC
		default:
			r.state = stateNormal
		}
		return false

	case stateCSI:
		if b >= 0x40 && b <= 0x7E {
			r.state = stateNormal
		}
		return false

	case stateOSC:
		if b == 0x07 {
			r.state = stateNormal
		} else if b == 0x1b {
			r.state = stateEsc
		}
		return false
	}

	return false
}

// pushClean writes a clean byte into the circular buffer and checks
// if the last len(prompt) bytes match the prompt.
func (r *Reader) pushClean(b byte) bool {
	r.window[r.pos] = b
	r.pos = (r.pos + 1) % len(r.window)
	if r.filled < len(r.window) {
		r.filled++
	}

	if r.filled < len(r.prompt) {
		return false
	}

	for i := range len(r.prompt) {
		idx := (r.pos + i) % len(r.window)
		if r.window[idx] != r.prompt[i] {
			return false
		}
	}
	return true
}

// WaitForPrompt reads from the source until the prompt is detected or
// the context is cancelled. Returns the full raw output.
func (r *Reader) WaitForPrompt(ctx context.Context) (string, error) {
	done := make(chan struct{})
	readErr := make(chan error, 1)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.src.Read(buf)
			for _, b := range buf[:n] {
				if r.feedByte(b) {
					close(done)
					return
				}
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	select {
	case <-done:
		return r.raw.String(), nil
	case err := <-readErr:
		return r.raw.String(), err
	case <-ctx.Done():
		return r.raw.String(), fmt.Errorf("waiting for prompt %q: %w", r.prompt, ctx.Err())
	}
}

// Reset clears the reader state so it can be reused for the next command
// on the same session.
// Panics if prompt is empty.
func (r *Reader) Reset(prompt string) {
	if len(prompt) == 0 {
		panic("cmdx: prompt must not be empty")
	}
	p := []byte(prompt)
	r.prompt = p
	r.window = make([]byte, len(p))
	r.pos = 0
	r.filled = 0
	r.state = stateNormal
	r.raw.Reset()
}
