package toolset

import (
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

func mustLocalExecutor(t testing.TB, root string) *fs.LocalExecutor {
	t.Helper()
	executor, err := fs.NewLocalExecutor(root)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func mustReadTool(t testing.TB, reader fs.Reader) *fs.ReadTool {
	t.Helper()
	tool, err := fs.NewReadTool(reader)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func mustApplyPatchTool(t testing.TB, executor fs.PatchApplier) *fs.ApplyPatchTool {
	t.Helper()
	tool, err := fs.NewApplyPatchTool(executor)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func mustRuntimeReadTool(t testing.TB, root string, reader fs.Reader) *fs.ReadTool {
	t.Helper()
	tool, err := newRuntimeReadTool(root, reader)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func mustDirectTools(t testing.TB, root string) []toolcontract.Tool {
	t.Helper()
	tools, err := directTools(root)
	if err != nil {
		t.Fatal(err)
	}
	return tools
}
