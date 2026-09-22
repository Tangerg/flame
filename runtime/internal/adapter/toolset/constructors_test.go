package toolset

import (
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

func mustLocalExecutor(t testing.TB, root string) *fs.LocalExecutor {
	t.Helper()
	executor, err := fs.NewLocalExecutor(mustRoot(t, root).Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executor.Close(); err != nil {
			t.Error(err)
		}
	})
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

func mustRuntimeReadTool(t testing.TB, reader fs.Reader) *fs.ReadTool {
	t.Helper()
	tool, err := newRuntimeReadTool(reader)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func mustDirectTools(t testing.TB, root string) []toolcontract.Tool {
	t.Helper()
	manifest, err := openDirectTools(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})
	return manifest.Visible
}

func mustRoot(t testing.TB, directory string) *filesystemRoot {
	t.Helper()
	root, err := openFilesystemRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	return root
}
