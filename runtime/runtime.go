package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/bootstrap"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

var (
	ErrClosed = errors.New("runtime: closed")
)

type Config struct {
	DataDirectory        string
	DefaultWorkspacePath string
	UserHomePath         string
	ConfigDirectories    []string
}

type Runtime struct {
	mu       sync.RWMutex
	stopping bool
	instance *bootstrap.Instance
}

func Open(ctx context.Context, cfg Config) (*Runtime, error) {
	resolved, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("runtime: locate distribution executable: %w", err)
	}
	firstPartyDirectory, err := localruntime.FirstPartyDirectory(executable)
	if err != nil {
		return nil, err
	}
	runnerExecutable, err := localruntime.RunnerExecutable(executable)
	if err != nil {
		return nil, err
	}
	instance, _, err := bootstrap.OpenInstance(ctx, bootstrap.InstanceConfig{
		UserHome:             resolved.UserHomePath,
		DefaultWorkspacePath: resolved.DefaultWorkspacePath,
		DataDirectory:        resolved.DataDirectory,
		FirstPartyDirectory:  firstPartyDirectory,
		RunnerExecutable:     runnerExecutable,
		ConfigDirectories:    resolved.ConfigDirectories,
	})
	if err != nil {
		return nil, err
	}
	return &Runtime{instance: instance}, nil
}

func (c Config) withDefaults() (Config, error) {
	if c.UserHomePath == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("runtime: locate user home: %w", err)
		}
		c.UserHomePath = userHome
	}
	if c.DefaultWorkspacePath == "" {
		c.DefaultWorkspacePath = c.UserHomePath
	}
	if len(c.ConfigDirectories) == 0 {
		c.ConfigDirectories = []string{c.DataDirectory}
	}
	return c, nil
}

func (r *Runtime) endpoint() (*delivery.Endpoint, error) {
	if r == nil {
		return nil, ErrClosed
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopping || r.instance == nil {
		return nil, ErrClosed
	}
	return r.instance.Endpoint(), nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.stopping = true
	instance := r.instance
	r.mu.Unlock()
	if instance == nil {
		return nil
	}
	return instance.Close()
}
