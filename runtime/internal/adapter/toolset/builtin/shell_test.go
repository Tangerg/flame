package builtin

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/content"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
	"github.com/Tangerg/flame/runtime/internal/infra/process/sandbox"
)

func TestShellToolsCannotReadOrStopAnotherSession(t *testing.T) {
	shells := unconfinedShells(t)
	owner := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "owner", CWD: t.TempDir()})
	other := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "other", CWD: t.TempDir()})
	started, err := callTextTool(owner, shellTool(t, shells, "shell"),
		`{"command":"sleep 30","description":"Keep session shell alive","run_in_background":true}`)
	if err != nil {
		t.Fatal(err)
	}
	id := backgroundShellID(t, started)
	sh, found := shells.Get("owner", id)
	if !found {
		t.Fatal("launch did not use its execution Session")
	}
	if _, err := sh.Write([]byte("private output")); err != nil {
		t.Fatal(err)
	}
	arguments := `{"shell_id":"` + id + `"}`
	for _, name := range []string{"read_shell_output", "stop_shell"} {
		result, err := callTextTool(other, shellTool(t, shells, name), arguments)
		if err != nil || !strings.Contains(result, "No background shell") {
			t.Fatalf("foreign %s = %q, %v; want unavailable shell", name, result, err)
		}
	}
	if finished, _ := sh.Status(); finished {
		t.Fatal("foreign stop terminated the owner's shell")
	}
	if _, err := callTextTool(owner, shellTool(t, shells, "stop_shell"), arguments); err != nil {
		t.Fatal(err)
	}
	<-sh.Done()
	result, err := callTextTool(owner, shellTool(t, shells, "read_shell_output"), arguments)
	if err != nil || !strings.Contains(result, "private output") {
		t.Fatalf("owner final read = %q, %v; foreign read consumed its output", result, err)
	}
}

func TestShellOutputsPreserveArbitraryBytes(t *testing.T) {
	for _, background := range []bool{false, true} {
		t.Run(map[bool]string{false: "foreground", true: "background"}[background], func(t *testing.T) {
			shells := unconfinedShells(t)
			command := `printf '\377\000\303'`
			var output string
			var err error
			if background {
				id, launchErr := shells.Launch(t.Context(), "", t.TempDir(), command, exec.Timeout{}, false)
				if launchErr != nil {
					t.Fatal(launchErr)
				}
				output, err = callTextTool(attachedRun(t), shellTool(t, shells, "read_shell_output"),
					`{"shell_id":"`+id+`","wait":true}`)
			} else {
				arguments, marshalErr := json.Marshal(shellArgs{Command: command, Description: "Print arbitrary bytes"})
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				output, err = callTextTool(attachedRun(t), shellTool(t, shells, "shell"), string(arguments))
			}
			if err != nil {
				t.Fatalf("shell output was lost: %v", err)
			}
			var result struct {
				Stdout content.Content `json:"stdout"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatalf("decode shell output %q: %v", output, err)
			}
			if got := result.Stdout.Bytes(); !bytes.Equal(got, []byte{0xff, 0, 0xc3}) {
				t.Fatalf("shell bytes = %x, want ff00c3", got)
			}
			if retained := shells.RetainedForSession(""); len(retained) != 0 {
				t.Fatal("completed shell remained after its output was published")
			}
		})
	}
}

func TestShellFailurePreservesObservedOutputThroughToolBinding(t *testing.T) {
	for _, cause := range []error{errors.New("process cleanup failed"), context.Canceled} {
		t.Run(cause.Error(), func(t *testing.T) {
			observed := exec.Output{
				Text: string([]byte{0xff, 0x00, 0xc3}), Finished: true,
				ExitCode: 3, Info: "exit 3", Duration: time.Second,
			}
			result, err := completedJSON(observed)
			if err != nil {
				t.Fatal(err)
			}
			executable, err := toolcontract.NewFunc[struct{}, string](
				toolcontract.FuncConfig{Name: "failed_shell", Description: "Return observed shell evidence"},
				func(context.Context, struct{}) (string, error) { return shellResult(result, cause) },
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = callTextTool(t.Context(), executable, `{}`)
			callErr, observedFailure := errors.AsType[*toolcontract.CallError](err)
			if !observedFailure || !errors.Is(err, cause) {
				t.Fatalf("Tool error = %v, want original cause and observed evidence", err)
			}
			if err := callErr.Validate(); err != nil {
				t.Fatal(err)
			}
			text, textual := callErr.Evidence().Text()
			if !textual {
				t.Fatal("shell failure lost its encoded result")
			}
			var evidence struct {
				Stdout   content.Content `json:"stdout"`
				ExitCode int             `json:"exit_code"`
			}
			if err := json.Unmarshal([]byte(text), &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence.ExitCode != observed.ExitCode || !bytes.Equal(evidence.Stdout.Bytes(), []byte(observed.Text)) {
				t.Fatalf("failure evidence = %+v, want observed exit and exact bytes", evidence)
			}
			if _, definite := errors.AsType[*toolcontract.Failure](err); definite {
				t.Fatal("observed output declared an uncertain command retry-safe")
			}
		})
	}
}

func shellIntPointer(value int) *int { return &value }

// shellTool returns the named tool from a freshly-built shell tool set.
func shellTool(t *testing.T, shells *exec.Shells, name string) toolcontract.Tool {
	t.Helper()
	tools, err := BuildShell(shells)
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

func unconfinedShells(t *testing.T) *exec.Shells {
	t.Helper()
	shells, err := exec.NewShells(nil, sandbox.ErrUnavailable, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})
	return shells
}

func backgroundShellID(t *testing.T, result string) string {
	t.Helper()
	var payload struct {
		ShellID string `json:"shell_id"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("decode background shell result %q: %v", result, err)
	}
	if payload.ShellID == "" {
		t.Fatalf("background shell result has no identity: %q", result)
	}
	return payload.ShellID
}

// TestShell_CompletesInline checks the foreground fast path: a quick command
// finishes within the auto-background window and returns its output + exit code
// inline (not as a background job).
func TestShell_CompletesInline(t *testing.T) {
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")

	out, err := callTextTool(attachedRun(t), shell, `{"command":"printf hello","description":"Print hello"}`)
	if err != nil {
		t.Fatalf("shell err = %v", err)
	}
	var res struct {
		Stdout   content.Content `json:"stdout"`
		ExitCode int             `json:"exit_code"`
	}
	if json.Unmarshal([]byte(out), &res) != nil || string(res.Stdout.Bytes()) != "hello" || res.ExitCode != 0 {
		t.Fatalf("result = %q, want {stdout:hello, exit_code:0}", out)
	}
	// A completed command is removed, not left as a background job.
	if running := shells.RetainedForSession(""); len(running) != 0 {
		t.Error("finished command should be removed from the shell set")
	}
}

func TestShellContractRejectsRemovedArguments(t *testing.T) {
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	for _, arguments := range []string{
		`{"command":"true","description":"Run true","timeout":1000}`,
		`{"command":"true","description":"Run true","run_in_background":true,"auto_background_after_seconds":1}`,
	} {
		if _, err := callTextTool(attachedRun(t), shell, arguments); err == nil {
			t.Fatalf("shell accepted removed arguments: %s", arguments)
		}
	}
	if _, err := callTextTool(attachedRun(t), output, `{"shell_id":"bg_1","block":true}`); err == nil {
		t.Fatal("read_shell_output accepted removed block argument")
	}
	if _, err := callTextTool(attachedRun(t), output, `{"shell_id":"bg_1","timeout_millis":1000}`); err == nil {
		t.Fatal("read_shell_output accepted timeout_millis without wait=true")
	}
}

func TestShellContractRejectsNumericAbsenceSentinels(t *testing.T) {
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	for _, arguments := range []string{
		`{"command":"true","description":"Run true","timeout_millis":0}`,
		`{"command":"true","description":"Run true","auto_background_after_seconds":0}`,
	} {
		if _, err := callTextTool(attachedRun(t), shell, arguments); err == nil {
			t.Fatalf("shell accepted zero-valued optional duration: %s", arguments)
		}
	}
	if _, err := callTextTool(attachedRun(t), output, `{"shell_id":"bg_1","wait":true,"timeout_millis":0}`); err == nil {
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
	// The schema refuses a zero before the Tool runs, so the argument helper
	// only has to keep a present value distinct from an absent one.
	if after, afterErr := (shellArgs{AutoBackgroundAfterSeconds: &zero}).autoBackgroundAfter(); afterErr != nil || after != 0 {
		t.Fatalf("present zero auto-background = %v, %v; want it preserved", after, afterErr)
	}

	maximumInt := int(^uint(0) >> 1)
	if int64(maximumInt) > int64(^uint64(0)>>1)/int64(time.Millisecond) {
		if _, err := optionalShellTimeout(&maximumInt, time.Millisecond, "timeout_millis"); err == nil {
			t.Fatal("timeout duration overflow was accepted")
		}
	}
}

func TestShellRequiresConciseDescription(t *testing.T) {
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")

	for _, arguments := range []string{
		`{"command":"true"}`,
		`{"command":"true","description":"   "}`,
		`{"command":"true","description":" Run tests"}`,
		`{"command":"true","description":"Run tests "}`,
		`{"command":"true","description":"` + strings.Repeat("x", 121) + `"}`,
	} {
		if _, err := callTextTool(attachedRun(t), shell, arguments); err == nil {
			t.Fatalf("shell accepted invalid description: %s", arguments)
		}
	}
}

func TestShellDescriptionSchemaIsRequiredAndBounded(t *testing.T) {
	shells := unconfinedShells(t)
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
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	// A completed command may correctly return inline even when background was
	// requested. Hold this one alive until the test owns its background handle.
	release := filepath.Join(t.TempDir(), "release")
	quotedRelease := "'" + strings.ReplaceAll(release, "'", "'\"'\"'") + "'"
	arguments, err := json.Marshal(shellArgs{
		Command:         "while [ ! -f " + quotedRelease + " ]; do sleep 0.01; done; printf hi",
		Description:     "Print hi after release",
		RunInBackground: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := callTextTool(attachedRun(t), shell, string(arguments))
	if err != nil {
		t.Fatalf("shell(bg) = %q err=%v", out, err)
	}
	id := backgroundShellID(t, out)
	// No exit_code while it's a live job.
	if strings.Contains(out, "exit_code") {
		t.Errorf("backgrounded result must omit exit_code: %q", out)
	}
	sh, ok := shells.Get("", id)
	if !ok {
		t.Fatalf("background shell %q should still be registered", id)
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	<-sh.Done()
	read, err := callTextTool(attachedRun(t), output, `{"shell_id":"`+id+`"}`)
	if err != nil || !strings.Contains(read, "hi") {
		t.Fatalf("read_shell_output = %q err=%v, want the command's output", read, err)
	}
	if _, retained := shells.Get("", id); retained {
		t.Fatal("finished background shell retained after its final output was read")
	}
	again, err := callTextTool(attachedRun(t), output, `{"shell_id":"`+id+`"}`)
	if err != nil || !strings.Contains(again, "No background shell") {
		t.Fatalf("read retired shell = %q, %v", again, err)
	}
}

// TestReadShellOutput_Wait blocks until a backgrounded command finishes, then
// returns its output + a finished status in a single call (the crush wait
// design — event-driven, no sleep poll loop).
func TestReadShellOutput_Wait(t *testing.T) {
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	out, err := callTextTool(attachedRun(t), shell, `{"command":"sleep 0.3; printf done","description":"Wait then print done","run_in_background":true}`)
	if err != nil {
		t.Fatalf("shell(bg) = %q err=%v", out, err)
	}
	id := backgroundShellID(t, out)
	// Without blocking it's still running; with block it waits to completion.
	read, err := callTextTool(attachedRun(t), output, `{"shell_id":"`+id+`","wait":true}`)
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
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")
	output := shellTool(t, shells, "read_shell_output")

	out, err := callTextTool(attachedRun(t), shell, `{"command":"sleep 30","description":"Keep a background shell running","run_in_background":true}`)
	if err != nil {
		t.Fatalf("shell(bg) err=%v", err)
	}
	id := backgroundShellID(t, out)
	read, err := callTextTool(attachedRun(t), output, `{"shell_id":"`+id+`","wait":true,"timeout_millis":1000}`)
	if err != nil {
		t.Fatalf("read_shell_output(wait,timeout_millis) err=%v, want graceful still-running", err)
	}
	if !strings.Contains(read, "still running") {
		t.Fatalf("read_shell_output(wait,timeout_millis) = %q, want a still-running status", read)
	}
	if _, retained := shells.Get("", id); !retained {
		t.Fatal("reading a live background shell released its handle")
	}
}

// TestShell_AutoBackground checks the promotion path: a command still running
// after auto_background_after_seconds seconds is moved to the background and stays
// addressable by its shell id.
func TestShell_AutoBackground(t *testing.T) {
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")

	out, err := callTextTool(attachedRun(t), shell, `{"command":"sleep 30","description":"Wait in the background","auto_background_after_seconds":1}`)
	if err != nil {
		t.Fatalf("shell(auto-bg) = %q err=%v", out, err)
	}
	id := backgroundShellID(t, out)
	if running, err := shells.Kill("", id); err != nil || !running {
		t.Fatalf("kill = (running=%v err=%v), want the backgrounded shell still running", running, err)
	}
}

func TestShellCanceledForegroundJoinsBeforeRemoval(t *testing.T) {
	shells := unconfinedShells(t)
	tools := &commandTools{shells: shells}
	ctx, cancel := context.WithCancel(attachedRun(t))
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
		if live := shells.RetainedForSession(""); len(live) == 1 {
			shell, ok := shells.Get("", live[0].ID)
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
	if _, err := running.Write([]byte("observed before cancellation")); err != nil {
		t.Fatal(err)
	}
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled shell error = %v, want context.Canceled", err)
	}
	callErr, observed := errors.AsType[*toolcontract.CallError](err)
	if !observed {
		t.Fatal("canceled foreground command lost its output evidence")
	}
	text, _ := callErr.Evidence().Text()
	var evidence struct {
		Stdout   content.Content `json:"stdout"`
		ExitCode int             `json:"exit_code"`
		Killed   bool            `json:"killed"`
	}
	if err := json.Unmarshal([]byte(text), &evidence); err != nil {
		t.Fatal(err)
	}
	if string(evidence.Stdout.Bytes()) != "exit -1\nobserved before cancellation" ||
		evidence.ExitCode != exec.NoExitStatus || !evidence.Killed {
		t.Fatalf("canceled output = %+v, want killed command and observed evidence", evidence)
	}
	select {
	case <-running.Done():
	default:
		t.Fatal("foreground shell was removed before process cleanup joined")
	}
	if live := shells.RetainedForSession(""); len(live) != 0 {
		t.Fatal("canceled foreground shell remained in the owner ledger")
	}
}

// TestReadShellOutput_UnknownShell reports an unknown id gracefully (not an error).
func TestReadShellOutput_UnknownShell(t *testing.T) {
	shells := unconfinedShells(t)
	output := shellTool(t, shells, "read_shell_output")

	miss, err := callTextTool(attachedRun(t), output, `{"shell_id":"bg_999"}`)
	if err != nil || !strings.Contains(miss, "No background shell") {
		t.Fatalf("read_shell_output(unknown) = %q err=%v", miss, err)
	}
}

// TestShellReportsACommandThatNeverStarted pins both halves of one rule: a
// command that failed to start did not start. Backgrounding it would send the
// model to poll a shell that never ran, and reporting exit_code -1 with no
// account of why leaves the only diagnostic the shell has unread.
func TestShellReportsACommandThatNeverStarted(t *testing.T) {
	for _, background := range []bool{false, true} {
		name := map[bool]string{false: "foreground", true: "background"}[background]
		t.Run(name, func(t *testing.T) {
			shells := unconfinedShells(t)
			tools, err := BuildShell(shells)
			if err != nil {
				t.Fatal(err)
			}
			var shell toolcontract.Tool
			for _, built := range tools {
				if built.Definition().Name == "shell" {
					shell = built
				}
			}

			arguments := `{"command":"printf hello","description":"Print hello"}`
			if background {
				arguments = `{"command":"printf hello","description":"Print hello","run_in_background":true}`
			}
			out, err := callTextTool(attachedRunIn(t, filepath.Join(t.TempDir(), "never-created")), shell, arguments)
			if err != nil {
				t.Fatalf("shell err = %v", err)
			}

			var result struct {
				Stdout   content.Content `json:"stdout"`
				ExitCode int             `json:"exit_code"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatalf("decode %q: %v", out, err)
			}
			if strings.Contains(string(result.Stdout.Bytes()), "running in background") {
				t.Fatalf("a command that never started was reported as backgrounded: %q", out)
			}
			if result.ExitCode != exec.NoExitStatus {
				t.Fatalf("exit_code = %d, want %d", result.ExitCode, exec.NoExitStatus)
			}
			if !strings.Contains(string(result.Stdout.Bytes()), "start failed") {
				t.Fatalf("result gives no account of the failure: %q", out)
			}
		})
	}
}

// A Tool executes inside a Run; without one there is no workspace to run in,
// and the shell must refuse rather than run in some process default.
func TestShellRefusesToRunWithoutAnAttachedRun(t *testing.T) {
	shells := unconfinedShells(t)
	shell := shellTool(t, shells, "shell")
	if _, err := callTextTool(t.Context(), shell, `{"command":"printf hello","description":"Print hello"}`); err == nil || !strings.Contains(err.Error(), "no attached Run workspace") {
		t.Fatalf("shell without a Run scope = %v, want a refusal", err)
	}
}
