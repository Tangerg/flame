package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/qimu/jsonvalue"
)

// hooksRelPath is the cascade filename. Global lives at ~/.flame/hooks.json; a
// project's lives at <dir>/.flame/hooks.json for any dir from the project root
// down to the cwd.
const hooksRelPath = ".flame/hooks.json"

func discover(ctx context.Context, input hookprovider.Discovery) ([]hook.Hook, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	cwd, root, home := filepath.Clean(input.CWD), filepath.Clean(input.ProjectRoot), input.Home
	var out []hook.Hook
	seen := make(map[string]struct{})
	add := func(path string, scope hook.Scope) error {
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
		if err := hook.ValidateHookCascade(len(out) + len(file.Hooks)); err != nil {
			return fmt.Errorf("hooks: load cascade after %q: %w", abs, err)
		}
		for _, wire := range file.Hooks {
			h := wire.rule()
			h.Scope = scope
			h.Source = abs
			out = append(out, h)
		}
		return nil
	}

	if home != "" {
		if err := add(filepath.Join(home, hooksRelPath), hook.ScopeGlobal); err != nil {
			return nil, err
		}
	}
	if input.IncludeProject {
		chain := []string{cwd}
		for dir := cwd; dir != root; {
			dir = filepath.Dir(dir)
			chain = append(chain, dir)
		}
		slices.Reverse(chain)
		for _, dir := range chain {
			if err := add(filepath.Join(dir, hooksRelPath), hook.ScopeProject); err != nil {
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
	Event         hook.Event `json:"event"`
	Matcher       string     `json:"matcher,omitempty"`
	Command       string     `json:"command,omitempty"`
	Inject        string     `json:"inject,omitempty"`
	TimeoutMillis int        `json:"timeoutMillis,omitzero"`
}

func (h hookWire) rule() hook.Hook {
	return hook.Hook{
		Event: h.Event, Matcher: h.Matcher, Command: h.Command,
		Inject: h.Inject, TimeoutMillis: h.TimeoutMillis,
	}
}

func readHooksFile(ctx context.Context, path string) (file hooksFile, loaded bool, err error) {
	if err := cancellation.Err(ctx); err != nil {
		return hooksFile{}, false, err
	}
	handle, opened, err := filesystem.OpenFile(path, hook.MaxConfigurationFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return hooksFile{}, false, cancellation.Err(ctx)
	}
	if errors.Is(err, filesystem.ErrFileTooLarge) {
		return hooksFile{}, false, fmt.Errorf("%w: %w", hook.ErrConfigurationTooLarge, errors.Join(err, cancellation.Err(ctx)))
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
		hook.MaxConfigurationFileBytes+1,
	))
	capacityErr := hook.ValidateConfigurationFileSize(int64(len(data)))
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
	if err := hook.ValidateHooksPerFile(len(file.Hooks)); err != nil {
		return hooksFile{}, false, err
	}
	for index, wire := range file.Hooks {
		hook := wire.rule()
		if err := hook.Validate(); err != nil {
			return hooksFile{}, false, fmt.Errorf("hook %d: %w", index, err)
		}
	}
	return file, true, nil
}
