package lsp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Cmd.Wait must not close RPC-owned parent ends while replies are still draining.
// Closing the parent's child-end copies after Start preserves EOF propagation.
func openStdio(cmd *exec.Cmd) (*pipeRWC, func() error, error) {
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	childStdin, stdin, err := os.Pipe()
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("lsp: stdin pipe: %w", err), stdout.Close(), childStdout.Close())
	}
	cmd.Stdin, cmd.Stdout = childStdin, childStdout
	closeChildCopies := sync.OnceValue(func() error { return errors.Join(childStdin.Close(), childStdout.Close()) })
	return &pipeRWC{out: stdout, in: stdin}, closeChildCopies, nil
}

type pipeRWC struct {
	out io.ReadCloser
	in  io.WriteCloser
}

func (p *pipeRWC) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *pipeRWC) Write(b []byte) (int, error) { return p.in.Write(b) }

func (p *pipeRWC) Close() error {
	return errors.Join(p.in.Close(), p.out.Close())
}

func closeUnstartedPipes(command string, pipes *pipeRWC, cause error) error {
	if err := pipes.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("lsp: close %s pipes after launch failure: %w", command, err))
	}
	return cause
}
