package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Executor abstracts command execution for testing
type Executor interface {
	// Run executes a command and returns an error if it fails
	Run(ctx context.Context, name string, args ...string) error

	// RunWithOutput executes a command and returns its stdout
	RunWithOutput(ctx context.Context, name string, args ...string) (string, error)

	// RunWithStdin executes a command with stdin input
	RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) error
}

// OSExecutor implements Executor using os/exec
type OSExecutor struct {
	verbose bool
	stdout  io.Writer
	stderr  io.Writer
}

// Option configures an OSExecutor
type Option func(*OSExecutor)

// WithVerbose enables verbose output
func WithVerbose(verbose bool) Option {
	return func(e *OSExecutor) {
		e.verbose = verbose
	}
}

// WithStdout sets the stdout writer
func WithStdout(w io.Writer) Option {
	return func(e *OSExecutor) {
		e.stdout = w
	}
}

// WithStderr sets the stderr writer
func WithStderr(w io.Writer) Option {
	return func(e *OSExecutor) {
		e.stderr = w
	}
}

// New creates a new OSExecutor with the given options
func New(opts ...Option) *OSExecutor {
	e := &OSExecutor{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (e *OSExecutor) Run(ctx context.Context, name string, args ...string) error {
	if e.verbose {
		fmt.Fprintf(e.stderr, "$ %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	return cmd.Run()
}

func (e *OSExecutor) RunWithOutput(ctx context.Context, name string, args ...string) (string, error) {
	if e.verbose {
		fmt.Fprintf(e.stderr, "$ %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

func (e *OSExecutor) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) error {
	if e.verbose {
		fmt.Fprintf(e.stderr, "$ %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	return cmd.Run()
}
