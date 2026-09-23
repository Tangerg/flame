package toolset

import (
	json "encoding/json/v2"
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/toolfailure"
)

// FileMutationReporter supplies prospective paths for approval, locking, and
// mutation guards. These paths do not acknowledge actual file effects.
type FileMutationReporter interface {
	MutationPaths(arguments []byte) ([]string, error)
}

var _ FileMutationReporter = (*fs.ApplyPatchTool)(nil)

func mutationPaths(tool toolcontract.Tool, invocation toolcontract.Invocation) ([]string, error) {
	var paths []string
	reporter, ok, err := toolcontract.Capability[FileMutationReporter](tool)
	if err != nil {
		return nil, err
	}
	if ok {
		reported, err := reporter.MutationPaths(invocation.Arguments())
		if err != nil {
			// A reporter that cannot read this call's own arguments is describing
			// what the model wrote, and every guard asks before the Tool runs, so
			// nothing happened that could be in doubt. Left unclassified it would
			// reach the Host as an operation of unknown outcome and settle the Run
			// tree as lost, which is how one malformed patch ends a Session.
			return nil, toolfailure.Definite(err)
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
