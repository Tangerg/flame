package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/infra/exec"
)

func shellIntPointer(value int) *int { return &value }

// shellTool returns the named tool from a freshly-built shell tool set.
func shellTool(t *testing.T, shells *exec.Shells, name string) toolcontract.Tool {
	t.Helper()
	tools, err := BuildShell(shells, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools {
		if tl.Definition().Name == name {
			return tl
		}
	}
	t.Fatalf("tool %q not built", name)
	return nil
}

func cleanupShells(t *testing.T, shells *exec.Shells) {
	t.Helper()
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})
}

func backgroundShellID(t *testing.T, result string) string {
	t.Helper()
	var payload struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("decode background shell result %q: %v", result, err)
	}
	_, suffix, found := strings.Cut(payload.Stdout, "shell ")
	if !found {
		t.Fatalf("background shell result has no identity: %q", payload.Stdout)
	}
	identity, _, found := strings.Cut(suffix, ".")
	if !found || identity == "" {
		t.Fatalf("background shell result has malformed identity: %q", payload.Stdout)
	}
	return identity
}

// TestShell_CompletesInline checks the foreground fast path: a quick command
// finishes within the auto-background window and returns its output + exit code
// inline (not as a background job).
func TestShell_CompletesInline(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")

	out, err := shell.Call(context.Background(), `{"command":"printf hello","description":"Print hello"}`)
	if err != nil {
		t.Fatalf("shell err = %v", err)
	}
	var res struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	if json.Unmarshal([]byte(out), &res) != nil || res.Stdout != "hello" || res.ExitCode != 0 {
		t.Fatalf("result = %q, want {stdout:hello, exit_code:0}", out)
	}
	// A completed command is removed, not left as a background job.
	if running := shells.RunningForSession(""); len(running) != 0 {
		t.Error("finished command should be removed from the shell set")
	}
}

func TestShellContractRejectsRemovedArguments(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	for _, arguments := range []string{
		`{"command":"true","description":"Run true","timeout":1000}`,
		`{"command":"true","description":"Run true","run_in_background":true,"auto_background_after_seconds":1}`,
	} {
		if _, err := shell.Call(t.Context(), arguments); err == nil {
			t.Fatalf("shell accepted removed arguments: %s", arguments)
		}
	}
	if _, err := output.Call(t.Context(), `{"shell_id":"bg_1","block":true}`); err == nil {
		t.Fatal("read_shell_output accepted removed block argument")
	}
	if _, err := output.Call(t.Context(), `{"shell_id":"bg_1","timeout_millis":1000}`); err == nil {
		t.Fatal("read_shell_output accepted timeout_millis without wait=true")
	}
}

func TestShellContractRejectsNumericAbsenceSentinels(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	for _, arguments := range []string{
		`{"command":"true","description":"Run true","timeout_millis":0}`,
		`{"command":"true","description":"Run true","auto_background_after_seconds":0}`,
	} {
		if _, err := shell.Call(t.Context(), arguments); err == nil {
			t.Fatalf("shell accepted zero-valued optional duration: %s", arguments)
		}
	}
	if _, err := output.Call(t.Context(), `{"shell_id":"bg_1","wait":true,"timeout_millis":0}`); err == nil {
		t.Fatal("read_shell_output accepted zero-valued optional timeout")
	}
}

func TestShellDurationValuesPreservePresence(t *testing.T) {
	disabled, err := (shellArgs{}).timeout()
	if err != nil {
		t.Fatal(err)
	}
	if _, enabled := disabled.Duration(); enabled {
		t.Fatal("omitted timeout unexpectedly enabled a hard deadline")
	}

	zero := 0
	if _, err := (shellArgs{TimeoutMillis: &zero}).timeout(); err == nil {
		t.Fatal("present zero timeout was treated as omission")
	}
	if _, err := (shellArgs{AutoBackgroundAfterSeconds: &zero}).autoBackgroundAfter(); err == nil {
		t.Fatal("present zero auto-background duration was treated as the default")
	}

	maximumInt := int(^uint(0) >> 1)
	if int64(maximumInt) > int64(^uint64(0)>>1)/int64(time.Millisecond) {
		if _, err := optionalShellTimeout(&maximumInt, time.Millisecond, "timeout_millis"); err == nil {
			t.Fatal("timeout duration overflow was accepted")
		}
	}
}

func TestShellRequiresConciseDescription(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")

	for _, arguments := range []string{
		`{"command":"true"}`,
		`{"command":"true","description":"   "}`,
		`{"command":"true","description":" Run tests"}`,
		`{"command":"true","description":"Run tests "}`,
		`{"command":"true","description":"` + strings.Repeat("x", 121) + `"}`,
	} {
		if _, err := shell.Call(t.Context(), arguments); err == nil {
			t.Fatalf("shell accepted invalid description: %s", arguments)
		}
	}
}

func TestShellDescriptionSchemaIsRequiredAndBounded(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")
	encoded := shell.Definition().InputSchema
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			MinLength int `json:"minLength"`
			MaxLength int `json:"maxLength"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	description, ok := schema.Properties["description"]
	if !ok || !slices.Contains(schema.Required, "description") || description.MinLength != 1 || description.MaxLength != 120 {
		t.Fatalf("description schema = required %v property %+v", schema.Required, description)
	}
}

// TestShell_RunInBackground checks the explicit-background path: the command
// returns a shell id immediately, and read_shell_output reads its output.
func TestShell_RunInBackground(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	out, err := shell.Call(context.Background(), `{"command":"printf hi","description":"Print hi","run_in_background":true}`)
	if err != nil {
		t.Fatalf("shell(bg) = %q err=%v", out, err)
	}
	id := backgroundShellID(t, out)
	// No exit_code while it's a live job.
	if strings.Contains(out, "exit_code") {
		t.Errorf("backgrounded result must omit exit_code: %q", out)
	}
	sh, ok := shells.Get(id)
	if !ok {
		t.Fatalf("background shell %q should still be registered", id)
	}
	<-sh.Done()
	read, err := output.Call(context.Background(), `{"shell_id":"`+id+`"}`)
	if err != nil || !strings.Contains(read, "hi") {
		t.Fatalf("read_shell_output = %q err=%v, want the command's output", read, err)
	}
}

// TestReadShellOutput_Wait blocks until a backgrounded command finishes, then
// returns its output + a finished status in a single call (the crush wait
// design — event-driven, no sleep poll loop).
func TestReadShellOutput_Wait(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	out, err := shell.Call(context.Background(), `{"command":"sleep 0.3; printf done","description":"Wait then print done","run_in_background":true}`)
	if err != nil {
		t.Fatalf("shell(bg) = %q err=%v", out, err)
	}
	id := backgroundShellID(t, out)
	// Without blocking it's still running; with block it waits to completion.
	read, err := output.Call(context.Background(), `{"shell_id":"`+id+`","wait":true}`)
	if err != nil {
		t.Fatalf("read_shell_output(wait) err=%v", err)
	}
	if !strings.Contains(read, "done") || !strings.Contains(read, "finished") {
		t.Fatalf("read_shell_output(wait) = %q, want finished output containing 'done'", read)
	}
}

// TestReadShellOutput_WaitTimeout returns the current still-running output (not an
// error) when timeout_millis elapses before the command exits.
func TestReadShellOutput_WaitTimeout(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	out, err := shell.Call(context.Background(), `{"command":"sleep 30","description":"Keep a background shell running","run_in_background":true}`)
	if err != nil {
		t.Fatalf("shell(bg) err=%v", err)
	}
	id := backgroundShellID(t, out)
	read, err := output.Call(context.Background(), `{"shell_id":"`+id+`","wait":true,"timeout_millis":1000}`)
	if err != nil {
		t.Fatalf("read_shell_output(wait,timeout_millis) err=%v, want graceful still-running", err)
	}
	if !strings.Contains(read, "still running") {
		t.Fatalf("read_shell_output(wait,timeout_millis) = %q, want a still-running status", read)
	}
}

// TestShell_AutoBackground checks the promotion path: a command still running
// after auto_background_after_seconds seconds is moved to the background and stays
// addressable by its shell id.
func TestShell_AutoBackground(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	shell := shellTool(t, shells, "shell")

	out, err := shell.Call(context.Background(), `{"command":"sleep 30","description":"Wait in the background","auto_background_after_seconds":1}`)
	if err != nil {
		t.Fatalf("shell(auto-bg) = %q err=%v", out, err)
	}
	id := backgroundShellID(t, out)
	if running, err := shells.Kill(id); err != nil || !running {
		t.Fatalf("kill = (running=%v err=%v), want the backgrounded shell still running", running, err)
	}
}

func TestShellCanceledForegroundJoinsBeforeRemoval(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	tools := &commandTools{shells: shells}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := tools.run(ctx, shellArgs{
			Command: "sleep 30", Description: "Wait for cancellation",
			AutoBackgroundAfterSeconds: shellIntPointer(30),
		})
		result <- err
	}()

	var running *exec.Shell
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if live := shells.RunningForSession(""); len(live) == 1 {
			shell, ok := shells.Get(live[0].ID)
			if ok {
				running = shell
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if running == nil {
		cancel()
		t.Fatal("foreground shell was not registered")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled shell error = %v, want context.Canceled", err)
	}
	select {
	case <-running.Done():
	default:
		t.Fatal("foreground shell was removed before process cleanup joined")
	}
	if live := shells.RunningForSession(""); len(live) != 0 {
		t.Fatal("canceled foreground shell remained in the owner ledger")
	}
}

// TestReadShellOutput_UnknownShell reports an unknown id gracefully (not an error).
func TestReadShellOutput_UnknownShell(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)
	output := shellTool(t, shells, "read_shell_output")

	miss, err := output.Call(context.Background(), `{"shell_id":"bg_999"}`)
	if err != nil || !strings.Contains(miss, "No background shell") {
		t.Fatalf("read_shell_output(unknown) = %q err=%v", miss, err)
	}
}
