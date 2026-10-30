package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
)

func TestShell_CommandReceivesStdin(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), commandRequest{
		Command: `grep -q '"event":"UserPromptSubmit"' && echo '{"injectContext":"saw-event"}'`,
		Input:   hook.Input{Event: hook.UserPromptSubmit},
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
	got := Shell{}.RunHookCommand(context.Background(), commandRequest{
		Command: `:`,
		Input:   hook.Input{Event: hook.UserPromptSubmit},
		Timeout: time.Second,
	})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.Decision.Verdict != hook.CommandAllow {
		t.Fatalf("verdict = %q, want %q", got.Decision.Verdict, hook.CommandAllow)
	}
}

func TestShell_Timeout(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), commandRequest{
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
	got := Shell{}.RunHookCommand(ctx, commandRequest{Command: ":", Timeout: time.Second})
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
			if err == nil || decision != (hook.CommandDecision{}) {
				t.Fatalf("decision = %+v, %v; want refusal without admitted material", decision, err)
			}
		})
	}
	for _, output := range []string{"", `{}`, `{"decision":"allow"}`} {
		decision, err := hookDecisionFromWire([]byte(output))
		if err != nil || decision.Verdict != hook.CommandAllow {
			t.Fatalf("explicit allow = %+v, %v", decision, err)
		}
	}
}

func TestShellBoundsCommandOutputAndRejectsMalformedDecision(t *testing.T) {
	t.Run("oversized stdout", func(t *testing.T) {
		got := Shell{}.RunHookCommand(t.Context(), commandRequest{
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
		got := Shell{}.RunHookCommand(t.Context(), commandRequest{
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
			got := Shell{}.RunHookCommand(t.Context(), commandRequest{
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
		input hook.Input
	}{
		{
			name: "prompt",
			input: hook.Input{
				Event:  hook.UserPromptSubmit,
				Prompt: strings.Repeat("p", (256<<10)+1),
			},
		},
		{
			name: "tool arguments",
			input: hook.Input{
				Event: hook.PreToolUse,
				Tool: &hook.ToolInput{
					Name: "shell", Arguments: strings.Repeat("a", (256<<10)+1),
				},
			},
		},
		{
			name: "tool result",
			input: hook.Input{
				Event: hook.PostToolUse,
				Tool: &hook.ToolInput{
					Name: "shell", Result: strings.Repeat("r", (128<<10)+1),
				},
			},
		},
		{
			name: "reason",
			input: hook.Input{
				Event: hook.Stop, Reason: strings.Repeat("e", (8<<10)+1),
			},
		},
		{
			name: "aggregate stdin",
			input: hook.Input{
				Event: hook.SessionStart, SessionID: strings.Repeat("s", (512<<10)+1),
			},
		},
		{
			name: "encoded stdin",
			input: hook.Input{
				Event: hook.SessionStart, SessionID: strings.Repeat("\x00", 100<<10),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Shell{}.RunHookCommand(t.Context(), commandRequest{
				Command: ":", Input: test.input, Timeout: time.Second,
			})
			if got.Err == nil || got.ExitCode != -1 {
				t.Fatalf("oversized %s result = %+v, want pre-spawn rejection", test.name, got)
			}
		})
	}
}

func TestHookInputWirePreservesBoundedProjectionMarkers(t *testing.T) {
	original := hook.Input{Event: hook.PostToolUse, Prompt: "prefix", PromptTruncated: true, Tool: &hook.ToolInput{Name: "shell", Result: "prefix", ResultTruncated: true}}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded hook.Input
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.PromptTruncated || decoded.Tool == nil || !decoded.Tool.ResultTruncated {
		t.Fatal("bounded input markers were lost")
	}
}

func TestShellOversizedExitTwoRetainsItsOriginalOutcome(t *testing.T) {
	result := (Shell{}).RunHookCommand(t.Context(), commandRequest{Command: `printf '{"injectContext":"'; head -c 70000 /dev/zero | tr '\000' x; printf '"}'; printf 'blocked' >&2; exit 2`, Input: hook.Input{Event: hook.PreToolUse}, Timeout: time.Second})
	if result.ExitCode != 2 || result.Stderr != "blocked" || result.Err == nil {
		t.Fatalf("original exit-two outcome=%+v", result)
	}
}

func TestShellMalformedDecisionRetainsItsDiagnostic(t *testing.T) {
	result := (Shell{}).RunHookCommand(t.Context(), commandRequest{Command: `printf 'not-json'`, Input: hook.Input{Event: hook.PreToolUse}, Timeout: time.Second})
	if result.ExitCode != 0 || result.Err == nil || result.Decision.Verdict.Valid() {
		t.Fatalf("malformed command outcome=%+v", result)
	}
}

func TestShellDecisionAdvicePreservesVerdicts(t *testing.T) {
	for _, verdict := range []hook.CommandVerdict{hook.CommandAllow, hook.CommandDeny, hook.CommandAsk} {
		result := (Shell{}).RunHookCommand(t.Context(), commandRequest{Command: `printf '%s' '{"decision":"` + string(verdict) + `","reason":"reviewed policy"}'`, Input: hook.Input{Event: hook.PreToolUse}, Timeout: time.Second})
		if result.Err != nil || result.Decision.Verdict != verdict {
			t.Fatalf("advice=%+v, want %s", result, verdict)
		}
	}
}

func TestShellExitTwoKeepsBinaryStderrPrintable(t *testing.T) {
	result := (Shell{}).RunHookCommand(t.Context(), commandRequest{Command: `printf '\377' >&2; exit 2`, Input: hook.Input{Event: hook.PreToolUse}, Timeout: time.Second})
	if result.ExitCode != 2 || !utf8.ValidString(result.Stderr) || !strings.Contains(result.Stderr, `\xff`) {
		t.Fatalf("binary stderr outcome=%+v", result)
	}
}

func TestHookDecisionPreservesNullInsideOpaqueArguments(t *testing.T) {
	decision, err := hookDecisionFromWire([]byte(`{"rewriteArguments":"{\"value\":null}"}`))
	if err != nil || decision.Verdict != hook.CommandAllow || decision.RewriteArguments != `{"value":null}` {
		t.Fatalf("opaque arguments = %+v, %v", decision, err)
	}
}

func TestShellTimeoutKillsDescendantProcessGroup(t *testing.T) {
	started := time.Now()
	got := Shell{}.RunHookCommand(t.Context(), commandRequest{
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
