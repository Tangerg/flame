package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFilesCountsWorkingFilesNotConflictStages(t *testing.T) {
	if !Available() {
		t.Skip("git executable is required")
	}
	dir := t.TempDir()
	var environment []string
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_") {
			environment = append(environment, variable)
		}
	}
	environment = append(environment, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	command := func(input string, arguments ...string) string {
		t.Helper()
		cmd := exec.Command("git", arguments...)
		cmd.Dir = dir
		cmd.Env = environment
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	command("", "init", "--quiet")
	object := command("conflicted content\n", "hash-object", "-w", "--stdin")
	command("100644 "+object+" 1\tconflict.txt\n"+
		"100644 "+object+" 2\tconflict.txt\n"+
		"100644 "+object+" 3\tconflict.txt\n", "update-index", "--index-info")
	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("conflicted content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "conflict.txt"} {
		files, err := ListFiles(context.Background(), dir, path, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 || files[0] != "conflict.txt" {
			t.Fatalf("ListFiles(%q) = %v, want one working file", path, files)
		}
	}
}
