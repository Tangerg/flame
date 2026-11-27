//go:build darwin

package sandbox

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	toolshell "github.com/Tangerg/scope/tools/shell"
)

func TestRestrictedWorkspaceRunsCheckWithoutHostAccess(t *testing.T) {
	source, scratch, private := t.TempDir(), t.TempDir(), t.TempDir()
	secret := filepath.Join(private, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("restricted check reached the network")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	script := `test "${GITHUB_TOKEN-unset}" = unset || exit 1
if /bin/cat "$1"; then exit 2; fi
if (printf changed > "$1"); then exit 3; fi
if /usr/bin/curl --max-time 2 --fail "$2"; then exit 4; fi
printf 'project check completed\n' > receipt.txt
`
	if err := os.WriteFile(filepath.Join(source, "check.sh"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_TOKEN", "fixture-token")
	workspace, err := New(t.Context(), Config{BaseDir: scratch, RestrictReads: true}, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := workspace.Shutdown(); err != nil {
			t.Error(err)
		}
	}()
	quote := func(arg string) string { return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'" }
	output, err := workspace.Run(t.Context(), toolshell.Input{
		Cmd: "/bin/sh check.sh " + quote(secret) + " " + quote(server.URL),
	})
	if err != nil || output.ExitCode != 0 {
		t.Fatalf("restricted project check failed: %+v %v", output, err)
	}
	path, err := workspace.Path()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := os.ReadFile(filepath.Join(path, "receipt.txt"))
	if err != nil || string(receipt) != "project check completed\n" {
		t.Fatalf("isolated check receipt = (%q, %v)", receipt, err)
	}
	kept, err := os.ReadFile(secret)
	if err != nil || string(kept) != "private" || strings.Contains(string(output.Stdout), "private") {
		t.Fatalf("restricted check accessed private material: %v", err)
	}
}

func TestSeatbeltRunnerConfinesWritesAndEnvironment(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	home := t.TempDir()
	secret := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	runner, err := platformRunner(Config{UserHome: home})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_SANDBOX_SECRET", "must-not-leak")
	out, err := runner.Run(t.Context(), workspace, toolshell.Input{
		Cmd: "printf inside > inside.txt; printf %s \"${FLAME_SANDBOX_SECRET-unset}\"; test ! -r " + strconv.Quote(secret),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("inside command failed: exit=%d stdout=%q stderr=%q", out.ExitCode, out.Stdout, out.Stderr)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "inside.txt"))
	if err != nil || string(content) != "inside" {
		t.Fatalf("inside write = (%q, %v); exit=%d stdout=%q stderr=%q", content, err, out.ExitCode, out.Stdout, out.Stderr)
	}
	if strings.Contains(string(out.Stdout), "must-not-leak") {
		t.Fatal("sandbox inherited a credential-like environment variable")
	}

	out, err = runner.Run(t.Context(), workspace, toolshell.Input{Cmd: "printf outside > " + strconv.Quote(outside)})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode == 0 {
		t.Fatalf("outside write unexpectedly succeeded: stdout=%q stderr=%q", out.Stdout, out.Stderr)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside write created a file: %v", err)
	}
}
