package sshx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"golang.org/x/crypto/ssh"
)

// Session represents an interactive SSH shell session with a network device.
type Session struct {
	cfg    Config
	client *ssh.Client
	sess   *ssh.Session
	stdin  io.WriteCloser
	stdout io.Reader
}

// NewSession connects to the device, opens an interactive shell with PTY,
// and reads until the first prompt (consuming any login banner).
func NewSession(cfg Config) (*Session, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	sshCfg := &ssh.ClientConfig{
		User: cfg.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.ConnectTimeout,
		Config: ssh.Config{
			Ciphers: cfg.Ciphers,
		},
	}

	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("cmdx: dial %s: %w", addr, err)
	}

	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("cmdx: new session: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, fmt.Errorf("cmdx: stdin pipe: %w", err)
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, fmt.Errorf("cmdx: stdout pipe: %w", err)
	}

	// Request PTY — network devices require an interactive terminal.
	if err := sess.RequestPty("xterm", 40, 512, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}); err != nil {
		sess.Close()
		client.Close()
		return nil, fmt.Errorf("cmdx: request pty: %w", err)
	}

	// Start interactive shell.
	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		return nil, fmt.Errorf("cmdx: start shell: %w", err)
	}

	s := &Session{
		cfg:    cfg,
		client: client,
		sess:   sess,
		stdin:  stdin,
		stdout: stdout,
	}

	// Consume the login banner by reading until the first prompt.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout)
	defer cancel()

	if _, err := s.readUntilPrompt(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("cmdx: waiting for initial prompt: %w", err)
	}

	return s, nil
}

// Run sends a command and waits for the prompt. Returns the output
// between the command echo and the prompt.
func (s *Session) Run(ctx context.Context, cmd string) (string, error) {
	if err := s.write(cmd + "\n"); err != nil {
		return "", fmt.Errorf("cmdx: write command: %w", err)
	}

	// Use command timeout if context has no deadline.
	ctx, cancel := s.withCommandTimeout(ctx)
	defer cancel()

	output, err := s.readUntilPrompt(ctx)
	if err != nil {
		return output, fmt.Errorf("cmdx: run %q: %w", cmd, err)
	}
	return output, nil
}

// Write sends a command without waiting for the prompt.
// Useful for commands like "quit", "reboot", or "commit".
func (s *Session) Write(cmd string) error {
	if err := s.write(cmd + "\n"); err != nil {
		return fmt.Errorf("cmdx: write %q: %w", cmd, err)
	}
	return nil
}

// SetPrompt changes the expected prompt for subsequent commands.
// Useful when the device changes prompt between contexts
// (e.g. Nokia 7360: "HOST>#" → "HOST>environment#").
func (s *Session) SetPrompt(prompt string) {
	s.cfg.Prompt = prompt
}

// Close terminates the SSH session and connection.
func (s *Session) Close() error {
	var errs []error
	if s.sess != nil {
		errs = append(errs, s.sess.Close())
	}
	if s.client != nil {
		errs = append(errs, s.client.Close())
	}
	return errors.Join(errs...)
}

// write sends raw bytes to the device stdin.
func (s *Session) write(data string) error {
	_, err := s.stdin.Write([]byte(data))
	return err
}

// readUntilPrompt creates a Reader and waits for the prompt.
func (s *Session) readUntilPrompt(ctx context.Context) (string, error) {
	r := NewReader(s.stdout, s.cfg.Prompt)
	return r.WaitForPrompt(ctx)
}

// withCommandTimeout returns a context with the command timeout applied,
// unless the parent context already has an earlier deadline.
func (s *Session) withCommandTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, s.cfg.CommandTimeout)
}
