package sandbox

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

type Command struct {
	Name string
	Args []string
	Env  []string
}

type Confiner struct {
	readOnlyPaths []string
	hiddenPaths   []string
}

func Available() bool { return checkBackend() == nil }

// NewConfiner fails closed with ErrUnavailable so a caller that asked for confinement is
// refused rather than silently running unconfined.
func NewConfiner(userHome string, readOnlyPaths []string) (*Confiner, error) {
	if err := checkBackend(); err != nil {
		return nil, err
	}
	hidden, err := resolveHiddenHome(userHome)
	if err != nil {
		return nil, err
	}
	readable, err := resolveReadOnlyPaths(readOnlyPaths)
	if err != nil {
		return nil, err
	}
	return &Confiner{readOnlyPaths: readable, hiddenPaths: []string{hidden}}, nil
}

func (c *Confiner) Confine(root, command string) (Command, error) {
	if c == nil || len(c.hiddenPaths) == 0 {
		return Command{}, errors.New("sandbox: constructed confiner is required")
	}
	if command == "" {
		return Command{}, errors.New("sandbox: command is required")
	}
	realRoot, err := (filesystem.DirectoryResolver{}).ResolveExistingDir(root)
	if err != nil {
		return Command{}, fmt.Errorf("sandbox: resolve root: %w", err)
	}
	return seatbeltCommand(realRoot, command, c.readOnlyPaths, c.hiddenPaths), nil
}

func resolveReadOnlyPaths(in []string) ([]string, error) {
	paths := make([]string, 0, len(in))
	for _, value := range in {
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("sandbox: read-only path %q must be absolute", value)
		}
		realPath, err := filepath.EvalSymlinks(value)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve read-only path %q: %w", value, err)
		}
		paths = append(paths, realPath)
	}
	return paths, nil
}

func resolveHiddenHome(userHome string) (string, error) {
	if userHome == "" {
		return "", errors.New("sandbox: user home is required")
	}
	if !filepath.IsAbs(userHome) {
		return "", errors.New("sandbox: user home must be absolute")
	}
	realHome, err := filepath.EvalSymlinks(userHome)
	if err != nil {
		return "", fmt.Errorf("sandbox: resolve user home %q: %w", userHome, err)
	}
	return realHome, nil
}
