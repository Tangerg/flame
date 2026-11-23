package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	toolshell "github.com/Tangerg/scope/tools/shell"
)

var (
	ErrShutdown    = errors.New("sandbox: workspace is shut down")
	ErrUnavailable = errors.New("sandbox: command isolation is unavailable on this platform")
)

type Config struct {
	BaseDir       string
	UserHome      string
	ReadOnlyPaths []string
}

type workspaceState uint8

const (
	workspaceRunning workspaceState = iota
	workspaceShutdown
)

type commandRunner interface {
	Run(ctx context.Context, dir string, input toolshell.Input) (toolshell.Output, error)
}

type Workspace struct {
	mu     sync.RWMutex
	dir    string
	runner commandRunner
	state  workspaceState
}

var _ toolshell.Executor = (*Workspace)(nil)

func New(ctx context.Context, config Config, source string) (*Workspace, error) {
	runner, err := platformRunner(config.UserHome, config.ReadOnlyPaths)
	if err != nil {
		return nil, err
	}
	return newWorkspace(ctx, config, source, runner)
}

func newWorkspace(ctx context.Context, config Config, source string, runner commandRunner) (_ *Workspace, err error) {
	if ctx == nil {
		return nil, errors.New("sandbox: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errors.New("sandbox: command runner is nil")
	}
	if config.BaseDir == "" {
		return nil, errors.New("sandbox: base directory is required")
	}
	if !filepath.IsAbs(config.BaseDir) {
		return nil, errors.New("sandbox: base directory must be absolute")
	}
	if mkdirAllErr := os.MkdirAll(config.BaseDir, filesystem.PrivateDirectoryMode); mkdirAllErr != nil {
		return nil, fmt.Errorf("sandbox: create base directory: %w", mkdirAllErr)
	}
	dir, err := os.MkdirTemp(config.BaseDir, "workspace-")
	if err != nil {
		return nil, fmt.Errorf("sandbox: create workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	if source != "" {
		if copyErr := copyTree(ctx, source, dir); copyErr != nil {
			return nil, fmt.Errorf("sandbox: copy source: %w", copyErr)
		}
	}
	cleanup = false
	return &Workspace{dir: dir, runner: runner}, nil
}

func (w *Workspace) Path() (string, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.state == workspaceShutdown {
		return "", ErrShutdown
	}
	return w.dir, nil
}

func (w *Workspace) Run(ctx context.Context, input toolshell.Input) (toolshell.Output, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.state == workspaceShutdown {
		return toolshell.Output{}, ErrShutdown
	}
	return w.runner.Run(ctx, w.dir, input)
}

func (w *Workspace) Shutdown() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state == workspaceShutdown {
		return nil
	}
	if err := os.RemoveAll(w.dir); err != nil {
		return fmt.Errorf("sandbox: remove workspace: %w", err)
	}
	w.state = workspaceShutdown
	return nil
}
