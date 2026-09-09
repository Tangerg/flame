// Package hooks discovers trusted hook configuration and adapts external shell
// commands to the typed Application hook runner.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/cancelread"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/project"
)

// hooksRelPath is the cascade filename. Global lives at ~/.flame/hooks.json; a
// project's lives at <dir>/.flame/hooks.json for any dir from the project root
// down to the cwd.
const hooksRelPath = ".flame/hooks.json"

// Load discovers and parses the hooks.json cascade for a working directory and
// returns every configured hook, each stamped with its scope and source path.
func Load(ctx context.Context, cwd, home string) ([]domainhooks.Hook, error) {
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
	cwd = filepath.Clean(cwd)

	var out []domainhooks.Hook
	seen := make(map[string]struct{})
	add := func(path string, scope domainhooks.Scope) error {
		if err := ctx.Err(); err != nil {
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
	TimeoutMillis int               `json:"timeoutMillis,omitempty"`
}

func (h hookWire) domain() domainhooks.Hook {
	return domainhooks.Hook{
		Event: h.Event, Matcher: h.Matcher, Command: h.Command,
		Inject: h.Inject, TimeoutMillis: h.TimeoutMillis,
	}
}

func readHooksFile(ctx context.Context, path string) (hooksFile, bool, error) {
	if cause := context.Cause(ctx); cause != nil {
		return hooksFile{}, false, cause
	}
	handle, opened, err := fileinput.Open(path, domainhooks.MaxConfigurationFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return hooksFile{}, false, nil
	}
	if errors.Is(err, fileinput.ErrNotRegular) {
		return hooksFile{}, false, errors.New("not a regular file")
	}
	if errors.Is(err, fileinput.ErrTooLarge) {
		return hooksFile{}, false, domainhooks.ValidateConfigurationFileSize(domainhooks.MaxConfigurationFileBytes + 1)
	}
	if err != nil {
		return hooksFile{}, false, err
	}
	defer func() { _ = handle.Close() }()
	data, err := io.ReadAll(io.LimitReader(
		cancelread.Reader(ctx, handle),
		domainhooks.MaxConfigurationFileBytes+1,
	))
	if err != nil {
		return hooksFile{}, false, err
	}
	if err := domainhooks.ValidateConfigurationFileSize(int64(len(data))); err != nil {
		return hooksFile{}, false, err
	}
	if err := fileinput.VerifyPathVersion(handle, opened, path); err != nil {
		if errors.Is(err, fileinput.ErrChanged) {
			return hooksFile{}, false, errors.New("configuration changed while it was being read")
		}
		return hooksFile{}, false, fmt.Errorf("verify configuration after reading: %w", err)
	}
	if len(data) == 0 {
		return hooksFile{}, false, nil
	}
	if !utf8.Valid(data) {
		return hooksFile{}, false, errors.New("configuration is not valid UTF-8")
	}
	var file hooksFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return hooksFile{}, false, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return hooksFile{}, false, errors.New("configuration contains multiple JSON values")
		}
		return hooksFile{}, false, fmt.Errorf("configuration contains trailing data: %w", err)
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
