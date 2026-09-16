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

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/toolarg"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

// FileMutationReporter is the optional Tool capability that names the paths one
// invocation would mutate, read from that invocation's JSON arguments.
//
// It is matched by dynamic type assertion through the decorator chain, so a
// consumer that declares its own copy of this method set keeps compiling and
// silently stops matching the moment either copy changes. Every consumer,
// inside this package or not, must compile against this declaration.
type FileMutationReporter interface {
	MutationPaths(arguments []byte) ([]string, error)
}

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
			return nil, toolarg.Unusable(err)
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

func (m applyPatchTool) MutationPaths(arguments []byte) ([]string, error) {
	var request fs.ApplyPatchRequest
	if err := json.Unmarshal(arguments, &request); err != nil {
		return nil, fmt.Errorf("decode apply_patch mutation paths: %w", err)
	}
	files, _, err := gitdiff.Parse(strings.NewReader(request.Patch))
	if err != nil {
		// The parse is how the guards learn which files this call would touch, so
		// an unreadable patch cannot be applied at all. The counts are what models
		// get wrong, and a parser that overruns one hunk body reports the next
		// header as a stray line, so name the likely cause instead of only the
		// line it stopped at.
		return nil, fmt.Errorf(
			"apply_patch: the patch could not be parsed, so the files it would change cannot be checked "+
				"and nothing was applied: %w. In each header @@ -oldStart,oldCount +newStart,newCount @@, "+
				"oldCount must equal the number of context and removed lines in that hunk and newCount the "+
				"number of context and added lines. Recount every hunk and call the tool again",
			err,
		)
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
