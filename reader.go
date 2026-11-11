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

const readChunkSize = 4096

// Reader processes bytes from an io.Reader (SSH stdout), strips ANSI
// escape sequences via a finite state machine, and detects a prompt
// using a sliding window implemented as a circular buffer.
//
// A Reader is meant to live as long as its source: a single goroutine
// (started on the first WaitForPrompt) reads src and hands chunks over a
// channel. Bytes are never read by anyone else, so a WaitForPrompt that
// times out does not lose or steal output from the next one, and bytes
// that arrive after a prompt are kept for the next call.
//
// A Reader is not safe for concurrent use.
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

	// Single reader goroutine and its hand-off.
	started bool
	chunks  chan []byte // unbuffered: the pump blocks until a chunk is consumed
	errc    chan error  // receives the source error once, after the last chunk
	srcErr  error       // sticky source error (e.g. io.EOF after Close)

	// Bytes already read from src but not consumed yet
	// (what came after the prompt in the last chunk).
	pending []byte
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

// feed processes a chunk. When the prompt is found, the bytes after it are
// kept in pending for the next WaitForPrompt and feed returns true.
func (r *Reader) feed(chunk []byte) bool {
	for i, b := range chunk {
		if r.feedByte(b) {
			if rest := chunk[i+1:]; len(rest) > 0 {
				r.pending = append(r.pending, rest...)
			}
			return true
		}
	}
	return false
}

// start launches the single goroutine that reads src for the lifetime of
// the Reader. It exits when src returns an error (io.EOF on session close).
func (r *Reader) start() {
	r.started = true
	r.chunks = make(chan []byte)
	r.errc = make(chan error, 1)

	go func(src io.Reader, chunks chan<- []byte, errc chan<- error) {
		for {
			buf := make([]byte, readChunkSize)
			n, err := src.Read(buf)
			if n > 0 {
				chunks <- buf[:n]
			}
			if err != nil {
				errc <- err
				return
			}
		}
	}(r.src, r.chunks, r.errc)
}

// WaitForPrompt reads until the prompt is detected or the context is
// cancelled. Returns the raw output accumulated since the last Reset
// (or since creation), including the prompt.
//
// On timeout or cancellation the source keeps being owned by this Reader:
// nothing is lost, and whatever is still arriving is delivered to the next
// WaitForPrompt.
func (r *Reader) WaitForPrompt(ctx context.Context) (string, error) {
	// Bytes left over from the previous call come first.
	if len(r.pending) > 0 {
		p := r.pending
		r.pending = nil
		if r.feed(p) {
			return r.raw.String(), nil
		}
	}

	if r.srcErr != nil {
		return r.raw.String(), r.srcErr
	}
	if !r.started {
		r.start()
	}

	for {
		select {
		case chunk := <-r.chunks:
			if r.feed(chunk) {
				return r.raw.String(), nil
			}
		case err := <-r.errc:
			// chunks is unbuffered, so every chunk was consumed before this.
			r.srcErr = err
			return r.raw.String(), err
		case <-ctx.Done():
			return r.raw.String(), fmt.Errorf("waiting for prompt %q: %w", r.prompt, ctx.Err())
		}
	}
}

// Reset clears the prompt detection state and the accumulated output so
// the Reader can be reused for the next command on the same session.
// Bytes already read but not consumed are kept.
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
