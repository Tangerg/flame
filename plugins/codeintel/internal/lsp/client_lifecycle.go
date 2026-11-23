package lsp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
	"github.com/sourcegraph/jsonrpc2"
)

const initializeTimeout = 30 * time.Second

const clientShutdownTimeout = 2 * time.Second

func startClient(ctx context.Context, spec ServerSpec, root string) (_ *client, err error) {
	workspace, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() {
		if workspace != nil {
			err = errors.Join(err, workspace.Close())
		}
	}()

	// The process belongs to the cached client, not this initialization call;
	// CommandContext would kill a healthy server when the handshake ctx ends.
	cmd := exec.Command(spec.Command, spec.Args...) //nolint:noctx
	cmd.Dir = root
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	procgroup.Prepare(cmd)

	pipes, closeChildCopies, err := openStdio(cmd)
	if err != nil {
		return nil, err
	}
	startErr := cmd.Start()
	childCloseErr := closeChildCopies()
	if startErr != nil {
		return nil, closeUnstartedPipes(spec.Command, pipes, errors.Join(fmt.Errorf("lsp: start %s: %w", spec.Command, startErr), childCloseErr))
	}
	stopProcess := sync.OnceValue(func() error {
		err := procgroup.Stop(cmd)
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	})
	wait := make(chan processEnd, 1)
	go func() {
		err := cmd.Wait()
		if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
			err = nil
		}
		// Server launchers may exit while their children retain the pipes.
		// Completion owns those descendants even without an explicit Close.
		wait <- processEnd{exitErr: err, cleanupErr: stopProcess()}
		close(wait)
	}()

	// WithoutCancel: the cached connection's read loop outlives this call but keeps
	// the trace values.
	shutdownBase := context.WithoutCancel(ctx)
	connCtx, cancel := context.WithCancel(shutdownBase)
	c := &client{
		workspace:    workspace,
		spec:         spec,
		stopProcess:  stopProcess,
		cancel:       cancel,
		wait:         wait,
		shutdownBase: shutdownBase,
		open:         map[string]openDoc{},
		diags:        map[string]diagSet{},
		updated:      make(chan struct{}),
	}
	workspace = nil
	stream := jsonrpc2.NewBufferedStream(pipes, lspObjectCodec{})
	c.conn = newConnection(connCtx, stream, jsonrpc2.AsyncHandler(c))
	if childCloseErr != nil {
		return c, fmt.Errorf("lsp: close inherited pipe copies: %w", childCloseErr)
	}

	initCtx, initCancel := context.WithTimeout(ctx, initializeTimeout)
	defer initCancel()
	if err := c.initialize(initCtx); err != nil {
		return c, err
	}
	return c, nil
}

func (c *client) initialize(ctx context.Context) error {
	var res struct {
		Capabilities struct {
			TextDocumentSync jsontext.Value `json:"textDocumentSync"`
			PositionEncoding string         `json:"positionEncoding"`
		} `json:"capabilities"`
	}
	params := initializeParams{
		ProcessID:        os.Getpid(),
		RootURI:          pathToURI(c.workspace.Name()),
		Capabilities:     defaultCapabilities(),
		WorkspaceFolders: c.workspaceFolders(),
	}
	if err := c.call(ctx, "initialize", params, &res); err != nil {
		return fmt.Errorf("lsp: initialize %s: %w", c.spec.Name, err)
	}
	if encoding := res.Capabilities.PositionEncoding; encoding != "" && encoding != positionEncodingUTF16 {
		return fmt.Errorf("lsp: server position encoding %q is unsupported; require %s", encoding, positionEncodingUTF16)
	}
	policy, err := negotiateDocumentSync(res.Capabilities.TextDocumentSync)
	if err != nil {
		return fmt.Errorf("lsp: server document synchronization: %w", err)
	}
	c.syncPolicy = policy
	if err := c.notify(ctx, "initialized", struct{}{}); err != nil {
		return fmt.Errorf("lsp: initialized %s: %w", c.spec.Name, err)
	}
	return nil
}

func (c *client) close() error {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(c.shutdownBase, clientShutdownTimeout)
		defer cancel()

		closeConnection := func() error {
			err := c.conn.Close()
			c.cancel()
			return err
		}
		// jsonrpc2 serializes writes without observing Call's context. Closing
		// stdio at the deadline releases a blocked write before shutdown can join.
		stopClose := context.AfterFunc(ctx, func() { closeConnection() })
		defer stopClose()

		// Protocol shutdown is advisory: a crashed or wedged server still needs
		// its local resources reclaimed below.
		_ = c.conn.Call(ctx, "shutdown", nil, nil)
		_ = c.conn.Notify(ctx, "exit", nil)

		var errs []error
		if err := closeConnection(); err != nil {
			errs = append(errs, fmt.Errorf("lsp: close %s connection: %w", c.spec.Name, err))
		}

		select {
		case end := <-c.wait:
			if end.exitErr != nil {
				c.processErr = fmt.Errorf("lsp: %s process ended: %w", c.spec.Name, end.exitErr)
			}
			if end.cleanupErr != nil {
				errs = append(errs, end.cleanupErr)
			}
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf(
				"lsp: graceful shutdown of %s exceeded %s: %w",
				c.spec.Name,
				clientShutdownTimeout,
				ctx.Err(),
			))
			if err := killAndJoinProcess(c.spec.Name, c.stopProcess, c.wait); err != nil {
				errs = append(errs, err)
			}
		}
		if c.workspace != nil {
			errs = append(errs, c.workspace.Close())
		}
		c.cleanupErr = errors.Join(errs...)
	})
	return errors.Join(c.processErr, c.cleanupErr)
}

func (c *client) retire() error {
	_ = c.close()
	if c.processErr != nil {
		if _, exited := errors.AsType[*exec.ExitError](c.processErr); !exited {
			return errors.Join(c.processErr, c.cleanupErr)
		}
	}
	return c.cleanupErr
}

func killAndJoinProcess(name string, stop func() error, wait <-chan processEnd) error {
	if err := stop(); err != nil {
		return fmt.Errorf("lsp: kill unresponsive %s: %w", name, err)
	}
	end := <-wait
	if end.exitErr != nil {
		if _, exited := errors.AsType[*exec.ExitError](end.exitErr); !exited {
			return errors.Join(end.exitErr, end.cleanupErr)
		}
	}
	return end.cleanupErr
}

func (c *client) workspaceFolders() []workspaceFolder {
	return []workspaceFolder{{URI: pathToURI(c.workspace.Name()), Name: filepath.Base(c.workspace.Name())}}
}
