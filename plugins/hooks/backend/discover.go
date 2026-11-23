package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"

	framevalue "github.com/Tangerg/frame/value"
)

func discover(ctx context.Context, input hookprovider.Discovery) ([]hook.Hook, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	cascade := hookCascade{seen: make(map[string]struct{})}
	if input.Home != "" {
		if err := cascade.add(ctx, filepath.Join(input.Home, hook.ConfigurationPath), hook.ScopeGlobal); err != nil {
			return nil, err
		}
	}
	if input.IncludeProject {
		for _, dir := range projectChain(filepath.Clean(input.CWD), filepath.Clean(input.ProjectRoot)) {
			if err := cascade.add(ctx, filepath.Join(dir, hook.ConfigurationPath), hook.ScopeProject); err != nil {
				return nil, err
			}
		}
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	return cascade.hooks, nil
}

type hookCascade struct {
	hooks []hook.Hook
	seen  map[string]struct{}
}

func (c *hookCascade) add(ctx context.Context, path string, scope hook.Scope) error {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	abs := filepath.Clean(path)
	if _, dup := c.seen[abs]; dup {
		return nil
	}
	c.seen[abs] = struct{}{}
	file, ok, err := readHooksFile(ctx, abs)
	if err != nil {
		return fmt.Errorf("hooks: load config %q: %w", abs, err)
	}
	if !ok {
		return nil
	}
	if err := hook.ValidateHookCascade(len(c.hooks) + len(file.Hooks)); err != nil {
		return fmt.Errorf("hooks: load cascade after %q: %w", abs, err)
	}
	for _, wire := range file.Hooks {
		h := wire.rule()
		h.Scope = scope
		h.Source = abs
		c.hooks = append(c.hooks, h)
	}
	return nil
}

func projectChain(cwd, root string) []string {
	chain := []string{cwd}
	for dir := cwd; dir != root; {
		dir = filepath.Dir(dir)
		chain = append(chain, dir)
	}
	slices.Reverse(chain)
	return chain
}

type hooksFile struct {
	Hooks []hookWire `json:"hooks"`
}

type hookWire struct {
	Event         hook.Event         `json:"event"`
	Matcher       string             `json:"matcher,omitempty"`
	Command       string             `json:"command,omitempty"`
	Inject        string             `json:"inject,omitempty"`
	TimeoutMillis int                `json:"timeoutMillis,omitzero"`
	FailurePolicy hook.FailurePolicy `json:"failurePolicy,omitempty"`
}

func (h hookWire) rule() hook.Hook {
	return hook.Hook{
		Event: h.Event, Matcher: h.Matcher, Command: h.Command,
		Inject: h.Inject, TimeoutMillis: h.TimeoutMillis, FailurePolicy: h.FailurePolicy,
	}
}

func readHooksFile(ctx context.Context, path string) (file hooksFile, loaded bool, err error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return hooksFile{}, false, err
	}
	handle, opened, err := filesystem.OpenFile(path, hook.MaxConfigurationFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return hooksFile{}, false, errors.Join(ctx.Err(), context.Cause(ctx))
	}
	if errors.Is(err, filesystem.ErrFileTooLarge) {
		return hooksFile{}, false, fmt.Errorf("%w: %w", hook.ErrConfigurationTooLarge, errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx))))
	}
	if err != nil {
		return hooksFile{}, false, errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx)))
	}
	defer func() {
		err = errors.Join(err, handle.Close())
		if err != nil {
			file = hooksFile{}
			loaded = false
		}
	}()
	contextReader, err := capture.NewReader(ctx, handle)
	if err != nil {
		return hooksFile{}, false, err
	}
	data, readErr := io.ReadAll(io.LimitReader(
		contextReader,
		hook.MaxConfigurationFileBytes+1,
	))
	capacityErr := hook.ValidateConfigurationFileSize(int64(len(data)))
	verifyErr := filesystem.VerifyPathVersion(handle, opened, path)
	if err := errors.Join(readErr, capacityErr, verifyErr, errors.Join(ctx.Err(), context.Cause(ctx))); err != nil {
		return hooksFile{}, false, fmt.Errorf("inspect configuration after reading: %w", err)
	}
	if len(data) == 0 {
		return hooksFile{}, false, nil
	}
	if err := framevalue.DecodeValue(data, &file, "hook configuration"); err != nil {
		return hooksFile{}, false, fmt.Errorf("%w: %w", hook.ErrInvalidHook, err)
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
