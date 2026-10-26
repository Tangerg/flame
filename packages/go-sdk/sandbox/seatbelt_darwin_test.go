//go:build darwin

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	toolshell "github.com/Tangerg/scope/tools/shell"
)

func TestSeatbeltRunnerSettlesCancellationAfterStoppingTheCommandGroup(t *testing.T) {
	runner, err := platformRunner(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name           string
		parentDeadline bool
	}{{"command timeout", false}, {"parent cancellation", true}} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			input := toolshell.Input{Cmd: "sleep 10 & wait", Timeout: 100 * time.Millisecond}
			if test.parentDeadline {
				ctx, cancel = context.WithTimeout(ctx, input.Timeout)
				defer cancel()
				input.Timeout = 0
			}
			out, err := runner.Run(ctx, t.TempDir(), input)
			if err != nil || !out.CancellationObserved || out.ExitCode != -1 {
				t.Fatalf("group cancellation did not settle its execution fact: %+v %v", out, err)
			}
		})
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
	runner, err := platformRunner(home, nil)
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
