package toolset

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
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

type applyPatchTool struct {
	toolcontract.Tool
}

func (m applyPatchTool) Definition() chat.ToolDefinition {
	definition := m.Tool.Definition()
	definition.Description += `

The patch argument is plain Git unified diff text, without Markdown fences or
*** Begin Patch / *** Update File markers. Paths are relative to the working
directory. Every hunk needs @@ -oldStart,oldCount +newStart,newCount @@: count
context and removed lines on the old side, context and added lines on the new
side. Prefix each context line with a space, removed line with -, and added
line with +. Include the final newline. Keep hunks small enough to count exactly.

Create notes.txt with one line:
` + "```diff\n" + `diff --git a/notes.txt b/notes.txt
new file mode 100644
--- /dev/null
+++ b/notes.txt
@@ -0,0 +1,1 @@
+first
` + "```\n" + `
After reading notes.txt, replace its line:
` + "```diff\n" + `diff --git a/notes.txt b/notes.txt
--- a/notes.txt
+++ b/notes.txt
@@ -1,1 +1,1 @@
-first
+second
` + "```\n" + `
After reading notes.txt, delete it:
` + "```diff\n" + `diff --git a/notes.txt b/notes.txt
deleted file mode 100644
--- a/notes.txt
+++ /dev/null
@@ -1,1 +0,0 @@
-second
` + "```"
	return definition
}

func (m applyPatchTool) Unwrap() toolcontract.Tool { return m.Tool }

func withApplyPatchMutationPaths(inner toolcontract.Tool) toolcontract.Tool {
	return applyPatchTool{Tool: inner}
}

func (m applyPatchTool) MutationPaths(invocation toolcontract.Invocation) ([]string, error) {
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
