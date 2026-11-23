package toolset

import (
	json "encoding/json/v2"
	"fmt"
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

// MutationPaths must be side-effect-free; its result never acknowledges actual
// file effects.
type FileMutationReporter interface {
	MutationPaths(arguments []byte) ([]string, error)
}

var _ FileMutationReporter = (*fs.ApplyPatchTool)(nil)

func mutationPaths(tool toolcontract.Tool, invocation toolcontract.Invocation) ([]string, error) {
	var paths []string
	reporter, ok := toolcontract.As[FileMutationReporter](tool)
	if ok {
		reported, err := reporter.MutationPaths(invocation.Arguments())
		if err != nil {
			return nil, err
		}
		paths = append(paths, reported...)
	}
	if len(paths) == 0 {
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(invocation.Arguments(), &a); err != nil {
			return nil, fmt.Errorf("read mutation path: %w", err)
		}
		if a.Path != "" {
			paths = append(paths, a.Path)
		}
	}
	return cleanPathList(paths), nil
}

func resolvedMutationPaths(tool toolcontract.Tool, invocation toolcontract.Invocation, root *filesystemRoot) ([]string, error) {
	paths, err := mutationPaths(tool, invocation)
	if err != nil {
		return nil, err
	}
	for i, path := range paths {
		resolved, err := resolveRootPath(root, path)
		if err != nil {
			return nil, err
		}
		paths[i] = resolved
	}
	return cleanPathList(paths), nil
}

func cleanPathList(paths []string) []string {
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}
