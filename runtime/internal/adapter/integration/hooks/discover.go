// Package hooks discovers trusted hook configuration and adapts external shell
// commands to the typed Application hook runner.
package hooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/project"
)

// hooksRelPath is the cascade filename. Global lives at ~/.flame/hooks.json; a
// project's lives at <dir>/.flame/hooks.json for any dir from the project root
// down to the cwd.
const hooksRelPath = ".flame/hooks.json"

// loadCascade discovers and parses the hooks.json cascade for a working directory and
// returns every configured hook, each stamped with its scope and source path.
func loadCascade(ctx context.Context, cwd, home string) ([]domainhooks.Hook, error) {
	return load(ctx, cwd, home, true)
}

// load can exclude project hooks at the trust boundary. An untrusted project's
// config is neither executed nor allowed to break otherwise-valid global hooks;
// management inspection calls Load and still validates the complete cascade.
func load(ctx context.Context, cwd, home string, includeProject bool) ([]domainhooks.Hook, error) {
	if cwd == "" {
		return nil, errors.New("hooks: cwd is required")
	}
	if !filepath.IsAbs(cwd) {
		return nil, errors.New("hooks: cwd must be absolute")
	}
	if home != "" && !filepath.IsAbs(home) {
		return nil, errors.New("hooks: home must be absolute")
	}
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	cwd = filepath.Clean(cwd)

	var out []domainhooks.Hook
	seen := make(map[string]struct{})
	add := func(path string, scope domainhooks.Scope) error {
		if err := cancellation.Err(ctx); err != nil {
			return err
		}
		abs := filepath.Clean(path)
		if _, dup := seen[abs]; dup {
			return nil
		}
		seen[abs] = struct{}{}
		file, ok, err := readHooksFile(ctx, abs)
		if err != nil {
			return fmt.Errorf("hooks: load config %q: %w", abs, err)
		}
		if !ok {
			return nil
		}
		if err := domainhooks.ValidateHookCascade(len(out) + len(file.Hooks)); err != nil {
			return fmt.Errorf("hooks: load cascade after %q: %w", abs, err)
		}
		for _, wire := range file.Hooks {
			h := wire.domain()
			h.Scope = scope
			h.Source = abs
			out = append(out, h)
		}
		return nil
	}

	if home != "" {
		if err := add(filepath.Join(home, hooksRelPath), domainhooks.ScopeGlobal); err != nil {
			return nil, err
		}
	}
	if includeProject {
		root, err := project.Root(cwd)
		if err != nil {
			return nil, fmt.Errorf("hooks: locate project root for %q: %w", cwd, err)
		}
		for _, dir := range project.Chain(cwd, root) {
			if err := add(filepath.Join(dir, hooksRelPath), domainhooks.ScopeProject); err != nil {
				return nil, err
			}
		}
	}
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// hooksFile is the validated wire shape of one hooks.json file.
type hooksFile struct {
	Hooks []hookWire `json:"hooks"`
}

type hookWire struct {
	Event         domainhooks.Event `json:"event"`
	Matcher       string            `json:"matcher,omitempty"`
	Command       string            `json:"command,omitempty"`
	Inject        string            `json:"inject,omitempty"`
	TimeoutMillis int               `json:"timeoutMillis,omitzero"`
}

func (h hookWire) domain() domainhooks.Hook {
	return domainhooks.Hook{
		Event: h.Event, Matcher: h.Matcher, Command: h.Command,
		Inject: h.Inject, TimeoutMillis: h.TimeoutMillis,
	}
}

func readHooksFile(ctx context.Context, path string) (file hooksFile, loaded bool, err error) {
	if err := cancellation.Err(ctx); err != nil {
		return hooksFile{}, false, err
	}
	handle, opened, err := filesystem.OpenFile(path, domainhooks.MaxConfigurationFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return hooksFile{}, false, cancellation.Err(ctx)
	}
	if errors.Is(err, filesystem.ErrFileTooLarge) {
		return hooksFile{}, false, fmt.Errorf("%w: %w", domainhooks.ErrConfigurationTooLarge, errors.Join(err, cancellation.Err(ctx)))
	}
	if err != nil {
		return hooksFile{}, false, errors.Join(err, cancellation.Err(ctx))
	}
	defer func() {
		err = errors.Join(err, handle.Close())
		if err != nil {
			file = hooksFile{}
			loaded = false
		}
	}()
	data, readErr := io.ReadAll(io.LimitReader(
		cancellation.Reader(ctx, handle),
		domainhooks.MaxConfigurationFileBytes+1,
	))
	capacityErr := domainhooks.ValidateConfigurationFileSize(int64(len(data)))
	verifyErr := filesystem.VerifyPathVersion(handle, opened, path)
	if err := errors.Join(readErr, capacityErr, verifyErr, cancellation.Err(ctx)); err != nil {
		return hooksFile{}, false, fmt.Errorf("inspect configuration after reading: %w", err)
	}
	if len(data) == 0 {
		return hooksFile{}, false, nil
	}
	if err := jsonvalue.UnmarshalNonNull(data, &file); err != nil {
		return hooksFile{}, false, err
	}
	if file.Hooks == nil {
		return hooksFile{}, false, errors.New("hooks array is required")
	}
	if err := domainhooks.ValidateHooksPerFile(len(file.Hooks)); err != nil {
		return hooksFile{}, false, err
	}
	for index, wire := range file.Hooks {
		hook := wire.domain()
		if err := hook.Validate(); err != nil {
			return hooksFile{}, false, fmt.Errorf("hook %d: %w", index, err)
		}
	}
	return file, true, nil
}
