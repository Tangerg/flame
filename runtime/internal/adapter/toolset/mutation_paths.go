package toolset

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/Tangerg/flame/runtime/internal/infra/pathidentity"
)

type fileMutationReporter interface {
	MutationPaths(invocation toolcontract.Invocation) ([]string, error)
}

func mutationPaths(tool toolcontract.Tool, invocation toolcontract.Invocation) ([]string, error) {
	var paths []string
	reporter, ok, err := toolcontract.Capability[fileMutationReporter](tool)
	if err != nil {
		return nil, err
	}
	if ok {
		reported, err := reporter.MutationPaths(invocation)
		if err != nil {
			return nil, err
		}
		paths = append(paths, reported...)
	}
	if len(paths) == 0 {
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(invocation.Arguments(), &a)
		if a.Path != "" {
			paths = append(paths, a.Path)
		}
	}
	return cleanPathList(paths), nil
}

func resolvedMutationPaths(tool toolcontract.Tool, invocation toolcontract.Invocation, cwd string) ([]string, error) {
	paths, err := mutationPaths(tool, invocation)
	if err != nil {
		return nil, err
	}
	for i, path := range paths {
		resolved, err := pathidentity.Canonical(cwd, path)
		if err != nil {
			return nil, err
		}
		paths[i] = resolved
	}
	return cleanPathList(paths), nil
}

type mutationPathTool struct {
	toolcontract.Tool
	report func(toolcontract.Invocation) ([]string, error)
}

func (m mutationPathTool) MutationPaths(invocation toolcontract.Invocation) ([]string, error) {
	return m.report(invocation)
}

func (m mutationPathTool) Unwrap() toolcontract.Tool { return m.Tool }

func withApplyPatchMutationPaths(inner toolcontract.Tool) toolcontract.Tool {
	return mutationPathTool{Tool: inner, report: applyPatchMutationPaths}
}

func applyPatchMutationPaths(invocation toolcontract.Invocation) ([]string, error) {
	var request fs.ApplyPatchRequest
	if err := json.Unmarshal(invocation.Arguments(), &request); err != nil {
		return nil, fmt.Errorf("decode apply_patch mutation paths: %w", err)
	}
	files, _, err := gitdiff.Parse(strings.NewReader(request.Patch))
	if err != nil {
		return nil, fmt.Errorf("parse apply_patch mutation paths: %w", err)
	}
	paths := make([]string, 0, len(files)*2)
	for _, file := range files {
		if file == nil {
			continue
		}
		paths = append(paths, normalizedPatchPath(file.OldName), normalizedPatchPath(file.NewName))
	}
	return cleanPathList(paths), nil
}

func normalizedPatchPath(path string) string {
	if path == "" || path == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

func cleanPathList(paths []string) []string {
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}
