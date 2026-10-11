package hooks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

func TestShell_CommandReceivesStdin(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), apphooks.CommandRequest{
		Command: `grep -q '"event":"UserPromptSubmit"' && echo '{"injectContext":"saw-event"}'`,
		Input:   domainhooks.Input{Event: domainhooks.UserPromptSubmit},
		Timeout: time.Second,
	})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if strings.TrimSpace(got.Decision.InjectContext) != "saw-event" {
		t.Fatalf("decision = %+v", got.Decision)
	}
}

func TestShell_EmptyOutputIsExplicitAllow(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), apphooks.CommandRequest{
		Command: `:`,
		Input:   domainhooks.Input{Event: domainhooks.UserPromptSubmit},
		Timeout: time.Second,
	})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.Decision.Verdict != apphooks.CommandAllow {
		t.Fatalf("verdict = %q, want %q", got.Decision.Verdict, apphooks.CommandAllow)
	}
}

func TestShell_Timeout(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), apphooks.CommandRequest{
		Command: `sleep 5`,
		Timeout: 40 * time.Millisecond,
	})
	if !got.TimedOut {
		t.Fatalf("TimedOut = false, err=%v", got.Err)
	}
	if !errors.Is(got.Err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want deadline cause", got.Err)
	}
}

func TestShellPreservesParentCancellationCause(t *testing.T) {
	cause := errors.New("owner retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	got := Shell{}.RunHookCommand(ctx, apphooks.CommandRequest{Command: ":", Timeout: time.Second})
	if !errors.Is(got.Err, context.Canceled) || !errors.Is(got.Err, cause) {
		t.Fatalf("canceled command error = %v, want cancellation and owner cause", got.Err)
	}
}

func TestHookDecisionRejectsInvalidFields(t *testing.T) {
	for _, output := range []string{
		`{"decision":""}`,
		`{"decision":null}`,
		`{"reason":null}`,
		`{"injectContext":null}`,
		`{"rewriteArguments":null}`,
	} {
		t.Run(output, func(t *testing.T) {
			decision, err := hookDecisionFromWire([]byte(output))
			if err == nil || decision != (apphooks.CommandDecision{}) {
				t.Fatalf("decision = %+v, %v; want refusal without admitted material", decision, err)
			}
		})
	}
	for _, output := range []string{"", `{}`, `{"decision":"allow"}`} {
		decision, err := hookDecisionFromWire([]byte(output))
		if err != nil || decision.Verdict != apphooks.CommandAllow {
			t.Fatalf("explicit allow = %+v, %v", decision, err)
		}
	}
}

func TestShellBoundsCommandOutputAndRejectsMalformedDecision(t *testing.T) {
	t.Run("oversized stdout", func(t *testing.T) {
		got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
			Command: `printf '{"injectContext":"'; head -c 70000 /dev/zero | tr '\000' x; printf '"}'`,
			Timeout: time.Second,
		})
		if got.Err == nil {
			t.Fatal("oversized hook stdout was accepted")
		}
		if got.Decision.InjectContext != "" {
			t.Fatalf("oversized stdout reached the decision: %d bytes", len(got.Decision.InjectContext))
		}
	})

	t.Run("bounded stderr", func(t *testing.T) {
		got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
			Command: `head -c 70000 /dev/zero | tr '\000' x >&2`,
			Timeout: time.Second,
		})
		if len(got.Stderr) > 64<<10 {
			t.Fatalf("hook stderr retained %d bytes, want at most 64 KiB", len(got.Stderr))
		}
	})

	for _, output := range []string{
		`not-json`,
		`null`,
		`{"decision":"unknown"}`,
		`{"unknown":true}`,
		`{"decision":"deny"} {"decision":"allow"}`,
	} {
		t.Run(output, func(t *testing.T) {
			got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
				Command: `printf '%s' ` + shellSingleQuote(output),
				Timeout: time.Second,
			})
			if got.Err == nil {
				t.Fatalf("malformed hook decision %q was silently accepted", output)
			}
		})
	}
}

func TestShellRejectsUnboundedCommandInput(t *testing.T) {
	tests := []struct {
		name  string
		input domainhooks.Input
	}{
		{
			name: "prompt",
			input: domainhooks.Input{
				Event:  domainhooks.UserPromptSubmit,
				Prompt: strings.Repeat("p", (256<<10)+1),
			},
		},
		{
			name: "tool arguments",
			input: domainhooks.Input{
				Event: domainhooks.PreToolUse,
				Tool: &domainhooks.ToolInput{
					Name: "shell", Arguments: strings.Repeat("a", (256<<10)+1),
				},
			},
		},
		{
			name: "tool result",
			input: domainhooks.Input{
				Event: domainhooks.PostToolUse,
				Tool: &domainhooks.ToolInput{
					Name: "shell", Result: strings.Repeat("r", (128<<10)+1),
				},
			},
		},
		{
			name: "reason",
			input: domainhooks.Input{
				Event: domainhooks.Stop, Reason: strings.Repeat("e", (8<<10)+1),
			},
		},
		{
			name: "aggregate stdin",
			input: domainhooks.Input{
				Event: domainhooks.SessionStart, SessionID: strings.Repeat("s", (512<<10)+1),
			},
		},
		{
			name: "encoded stdin",
			input: domainhooks.Input{
				Event: domainhooks.SessionStart, SessionID: strings.Repeat("\x00", 100<<10),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
				Command: ":", Input: test.input, Timeout: time.Second,
			})
			if got.Err == nil || got.ExitCode != -1 {
				t.Fatalf("oversized %s result = %+v, want pre-spawn rejection", test.name, got)
			}
		})
	}
}

func TestHookInputWirePreservesBoundedProjectionMarkers(t *testing.T) {
	prompt := hookInputWireFrom(domainhooks.Input{
		Event: domainhooks.UserPromptSubmit, Prompt: "prefix", PromptTruncated: true,
	})
	if !prompt.PromptTruncated {
		t.Fatal("prompt truncation marker was dropped at the process boundary")
	}
	tool := hookInputWireFrom(domainhooks.Input{
		Event: domainhooks.PostToolUse,
		Tool:  &domainhooks.ToolInput{Name: "shell", Result: "prefix", ResultTruncated: true},
	})
	if tool.Tool == nil || !tool.Tool.ResultTruncated {
		t.Fatal("tool result truncation marker was dropped at the process boundary")
	}
}

func TestShellOversizedExitTwoRemainsAnExplicitDeny(t *testing.T) {
	runner := apphooks.NewRunner(Shell{}, nil)
	decision := runner.Run(t.Context(), []domainhooks.Hook{{
		Event: domainhooks.PreToolUse, Source: "/tmp/hooks.json",
		Command: `printf '{"injectContext":"'; head -c 70000 /dev/zero | tr '\000' x; printf '"}'; printf 'blocked' >&2; exit 2`,
	}}, domainhooks.Input{
		Event: domainhooks.PreToolUse,
		Tool:  &domainhooks.ToolInput{Name: "shell"},
	})
	if decision.Verdict != domainhooks.VerdictBlock || decision.Reason != "blocked" {
		t.Fatalf("oversized exit-two decision = %+v, want explicit bounded deny", decision)
	}
}

func TestShellMalformedDecisionIsObservableAndNonBlocking(t *testing.T) {
	var observed error
	runner := apphooks.NewRunner(Shell{}, func(_ context.Context, _ string, err error) {
		observed = err
	})
	decision := runner.Run(t.Context(), []domainhooks.Hook{{
		Event: domainhooks.PreToolUse, Source: "/tmp/hooks.json",
		Command: `printf 'not-json'`,
	}}, domainhooks.Input{
		Event: domainhooks.PreToolUse,
		Tool:  &domainhooks.ToolInput{Name: "shell"},
	})
	if decision.Verdict == domainhooks.VerdictBlock || observed == nil {
		t.Fatalf("malformed decision = %+v observed=%v, want observable non-blocking failure", decision, observed)
	}
}

func TestShellDecisionsReachTheApplicationPolicy(t *testing.T) {
	for _, test := range []struct {
		output  string
		verdict domainhooks.Verdict
	}{
		{`{"decision":"deny","reason":"reviewed policy"}`, domainhooks.VerdictBlock},
		{`{"decision":"ask","reason":"reviewed policy"}`, domainhooks.VerdictAsk},
	} {
		t.Run(test.output, func(t *testing.T) {
			var observed error
			runner := apphooks.NewRunner(Shell{}, func(_ context.Context, _ string, err error) { observed = err })
			decision := runner.Run(t.Context(), []domainhooks.Hook{{
				Event: domainhooks.PreToolUse, Command: `printf '%s' ` + shellSingleQuote(test.output),
			}}, domainhooks.Input{Event: domainhooks.PreToolUse, Tool: &domainhooks.ToolInput{Name: "shell"}})
			if observed != nil || decision.Verdict != test.verdict || decision.Reason != "reviewed policy" {
				t.Fatalf("command decision = %+v, %v", decision, observed)
			}
		})
	}
}

func TestShellExitTwoKeepsBinaryStderrPrintable(t *testing.T) {
	runner := apphooks.NewRunner(Shell{}, nil)
	decision := runner.Run(t.Context(), []domainhooks.Hook{{
		Event: domainhooks.PreToolUse, Command: `printf '\377' >&2; exit 2`,
	}}, domainhooks.Input{Event: domainhooks.PreToolUse, Tool: &domainhooks.ToolInput{Name: "shell"}})
	if decision.Verdict != domainhooks.VerdictBlock || !utf8.ValidString(decision.Reason) || !strings.Contains(decision.Reason, `\xff`) {
		t.Fatalf("binary stderr decision = %+v, want printable explicit deny", decision)
	}
}

func TestHookDecisionPreservesNullInsideOpaqueArguments(t *testing.T) {
	decision, err := hookDecisionFromWire([]byte(`{"rewriteArguments":"{\"value\":null}"}`))
	if err != nil || decision.Verdict != apphooks.CommandAllow || decision.RewriteArguments != `{"value":null}` {
		t.Fatalf("opaque arguments = %+v, %v", decision, err)
	}
}

func TestShellTimeoutKillsDescendantProcessGroup(t *testing.T) {
	started := time.Now()
	got := Shell{}.RunHookCommand(t.Context(), apphooks.CommandRequest{
		Command: `sleep 5 & wait`,
		Timeout: 40 * time.Millisecond,
	})
	if !got.TimedOut {
		t.Fatalf("TimedOut = false, err=%v", got.Err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timed-out hook retained a descendant process for %s", elapsed)
	}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
