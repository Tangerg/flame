package toolset

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
	"github.com/Tangerg/scope/tools/fs"
)

// openDirectTools owns the small, read-only capability set valid without an agent
// process. Keep this list explicit: being available to a model does not make a
// tool valid for a client-driven call.
func openDirectTools(root string) (_ Manifest, err error) {
	executor, err := fs.NewLocalExecutor(root)
	if err != nil {
		return Manifest{}, fmt.Errorf("toolset: construct direct filesystem executor: %w", err)
	}
	close := sync.OnceValue(executor.Close)
	defer func() {
		if err != nil {
			err = errors.Join(err, close())
		}
	}()
	readTool, err := newRuntimeReadTool(executor)
	if err != nil {
		return Manifest{}, err
	}
	search := newRuntimeSearchTools(root)
	return Manifest{Visible: []toolcontract.Tool{readTool, search.glob, search.grep}, close: close}, nil
}

// Only Scope-admitted inputs may be normalized: typed decoding and re-encoding
// can erase invalid zero/null values or normalize field aliases before admission.
// This adapter owns path identity and domain-error translation; LocalExecutor
// independently enforces the filesystem capability.
func normalizeDirectArguments(root, name string, invocation toolcontract.Invocation) (string, error) {
	switch name {
	case tool.Read:
		request, err := decodeToolArguments[fs.ReadRequest](invocation)
		if err != nil {
			return "", fmt.Errorf("toolset: decode direct read arguments: %w", err)
		}
		path, err := directPath(root, request.Path)
		if err != nil {
			return "", err
		}
		request.Path = path
		return encodeDirectArguments(request)
	case tool.Glob:
		request, err := decodeToolArguments[runtimeGlobRequest](invocation)
		if err != nil {
			return "", fmt.Errorf("toolset: decode direct glob arguments: %w", err)
		}
		if request.Path != "" {
			path, err := directPath(root, request.Path)
			if err != nil {
				return "", err
			}
			request.Path = path
		}
		return encodeDirectArguments(request)
	case tool.Grep:
		request, err := decodeToolArguments[runtimeGrepRequest](invocation)
		if err != nil {
			return "", fmt.Errorf("toolset: decode direct grep arguments: %w", err)
		}
		if request.Path != "" {
			path, err := directPath(root, request.Path)
			if err != nil {
				return "", err
			}
			request.Path = path
		}
		return encodeDirectArguments(request)
	default:
		return "", fmt.Errorf("toolset: direct tool %q is not registered", name)
	}
}

func decodeToolArguments[T any](invocation toolcontract.Invocation) (T, error) {
	var request T
	err := jsonv2.Unmarshal(invocation.Arguments(), &request, jsonv2.RejectUnknownMembers(true))
	return request, err
}

func encodeDirectArguments(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("toolset: encode direct arguments: %w", err)
	}
	return string(encoded), nil
}

func directPath(root, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%w: path is required", workspaceapp.ErrPathRequired)
	}
	// Resolve both values first. On macOS, temporary directories commonly have
	// a lexical /var/... spelling but a physical /private/var/... spelling;
	// comparing only a resolved target to an unresolved root would reject an
	// in-root file (or make the policy platform-dependent).
	resolvedRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		return "", fmt.Errorf("%w: resolve root %q: %w", workspaceapp.ErrPathOutsideRoot, root, err)
	}
	resolved, err := pathidentity.Resolve(resolvedRoot, path)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %w", workspaceapp.ErrPathOutsideRoot, path, err)
	}
	inside, err := pathidentity.Contains(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("%w: compare %q: %w", workspaceapp.ErrPathOutsideRoot, path, err)
	}
	if !inside {
		return "", fmt.Errorf("%w: %q", workspaceapp.ErrPathOutsideRoot, path)
	}
	relative, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", workspaceapp.ErrPathOutsideRoot, path, err)
	}
	return relative, nil
}

// directResult preserves a tool's structured JSON output when present and
// otherwise exposes its raw textual result as a JSON string, matching the
// protocol's best-effort JSON contract.
func directResult(output chat.ToolOutput) (tool.Result, error) {
	if err := output.Validate(); err != nil {
		return tool.Result{}, fmt.Errorf("toolset: invalid direct tool output: %w", err)
	}
	if len(output.Details) > 0 {
		result, err := tool.ParseResult(output.Details)
		if err != nil {
			return tool.Result{}, fmt.Errorf("toolset: decode direct tool details: %w", err)
		}
		return result, nil
	}
	text, textual := output.Text()
	if !textual {
		return tool.Result{}, errors.New("toolset: direct tool returned unsupported media output")
	}
	if result, err := tool.ParseResult([]byte(text)); err == nil {
		return result, nil
	}
	return tool.StringResult(text), nil
}
