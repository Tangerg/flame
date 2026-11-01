package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
)

func ctxBG() context.Context { return context.Background() }

type commandStub struct {
	mu       sync.Mutex
	results  []commandResult
	requests []commandRequest
}

func (c *commandStub) RunHookCommand(_ context.Context, req commandRequest) commandResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if req.Input.Tool != nil {
		req.Input.Tool = new(*req.Input.Tool)
	}
	c.requests = append(c.requests, req)
	if len(c.results) == 0 {
		return commandResult{}
	}
	out := c.results[0]
	c.results = c.results[1:]
	return out
}

func (c *commandStub) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func TestRunner_DeclarativeInject(t *testing.T) {
	r := newPolicyEvaluator(nil, nil)
	hooks := []hookdata.Hook{{Event: hookdata.SessionStart, Inject: "remember: use tabs"}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.SessionStart})
	if dec.InjectContext != "remember: use tabs" {
		t.Fatalf("InjectContext = %q", dec.InjectContext)
	}
	if dec.Verdict == hookdata.CommandDeny {
		t.Error("declarative inject should not block")
	}
}

func TestRunnerTreatsAbsentCommandRunnerAsDeclarativeOnly(t *testing.T) {
	for _, commands := range []commandRunner{nil, (*commandStub)(nil)} {
		var observed error
		runner := newPolicyEvaluator(commands, func(_ context.Context, _ string, err error) { observed = err })
		decision := runner.Run(t.Context(), []hookdata.Hook{
			{Event: hookdata.SessionStart, Command: "hook"},
			{Event: hookdata.SessionStart, Inject: "retained context"},
		}, hookdata.Input{Event: hookdata.SessionStart})
		if observed == nil || decision.InjectContext != "retained context" || decision.Verdict == hookdata.CommandDeny {
			t.Fatalf("declarative-only execution = %+v, %v", decision, observed)
		}
	}
}

func TestRunner_CommandReceivesTypedEvent(t *testing.T) {
	cmds := &commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow, InjectContext: "saw-event"}}}}
	r := newPolicyEvaluator(cmds, nil)
	hooks := []hookdata.Hook{{
		Event:   hookdata.UserPromptSubmit,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.UserPromptSubmit, Prompt: "hi"})
	if dec.InjectContext != "saw-event" {
		t.Fatalf("InjectContext = %q — stdin event not delivered?", dec.InjectContext)
	}
	if len(cmds.requests) != 1 || cmds.requests[0].Input.Event != hookdata.UserPromptSubmit || cmds.requests[0].Input.Prompt != "hi" {
		t.Fatalf("request = %+v, want typed prompt event", cmds.requests)
	}
}

func TestRunnerProjectsBoundedMaterialForCommandHooks(t *testing.T) {
	cmds := &commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow}}}}
	r := newPolicyEvaluator(cmds, nil)
	r.Run(ctxBG(), []hookdata.Hook{{
		Event: hookdata.UserPromptSubmit, Command: "hook",
	}}, hookdata.Input{
		Event:  hookdata.UserPromptSubmit,
		Prompt: strings.Repeat("p", hookdata.MaxPromptBytes+1),
	})
	if len(cmds.requests) != 1 {
		t.Fatalf("command requests = %d, want 1", len(cmds.requests))
	}
	got := cmds.requests[0].Input
	if len(got.Prompt) != hookdata.MaxPromptBytes || !got.PromptTruncated {
		t.Fatalf("projected prompt = %d bytes truncated=%v", len(got.Prompt), got.PromptTruncated)
	}
}

func TestRunnerRejectsLossyArgumentsWithoutDroppingDeclarativeContext(t *testing.T) {
	cmds := &commandStub{}
	var observed error
	r := newPolicyEvaluator(cmds, func(_ context.Context, _ string, err error) {
		observed = err
	})
	decision := r.Run(ctxBG(), []hookdata.Hook{
		{Event: hookdata.PreToolUse, Inject: "bounded context"},
		{Event: hookdata.PreToolUse, Command: "hook", Source: "/tmp/hooks.json"},
	}, hookdata.Input{
		Event: hookdata.PreToolUse,
		Tool: &hookdata.ToolInput{
			Name: "shell", Arguments: strings.Repeat("a", hookdata.MaxArgumentsBytes+1),
		},
	})
	if cmds.calls() != 0 || observed == nil {
		t.Fatalf("command calls = %d observed=%v, want rejected observable command input", cmds.calls(), observed)
	}
	if decision.InjectContext != "bounded context" {
		t.Fatalf("declarative context = %q, want preserved bounded path", decision.InjectContext)
	}
}

func TestRunner_StdoutDenyBlocks(t *testing.T) {
	r := newPolicyEvaluator(&commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandDeny, Reason: "no rm allowed"}}}}, nil)
	hooks := []hookdata.Hook{{
		Event:   hookdata.PreToolUse,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if dec.Verdict != hookdata.CommandDeny || dec.Reason != "no rm allowed" {
		t.Fatalf("got verdict=%v reason=%q, want deny", dec.Verdict, dec.Reason)
	}
}

func TestRunner_Exit2Blocks(t *testing.T) {
	r := newPolicyEvaluator(&commandStub{results: []commandResult{{
		Stderr:   "blocked by policy",
		ExitCode: blockExitCode,
		Err:      errors.New("exit status 2"),
	}}}, nil)
	hooks := []hookdata.Hook{{
		Event:   hookdata.PreToolUse,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if dec.Verdict != hookdata.CommandDeny || dec.Reason != "blocked by policy" {
		t.Fatalf("got verdict=%v reason=%q, want exit-2 block w/ stderr reason", dec.Verdict, dec.Reason)
	}
}

func TestRunnerAdmitsCommandMaterialFromTheProcessOutcome(t *testing.T) {
	for _, test := range []struct {
		exitCode int
		verdict  hookdata.CommandVerdict
		failure  bool
	}{
		{2, hookdata.CommandDeny, false},
		{3, hookdata.CommandAllow, true},
	} {
		var observed error
		runner := newPolicyEvaluator(&commandStub{results: []commandResult{{
			ExitCode: test.exitCode, Stderr: "process diagnostic",
			Decision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow, RewriteArguments: `{"unadmitted":true}`},
		}}}, func(_ context.Context, _ string, err error) { observed = err })
		decision := runner.Run(t.Context(), []hookdata.Hook{{Event: hookdata.PreToolUse, Command: "hook"}}, hookdata.Input{
			Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"},
		})
		if decision.Verdict != test.verdict || decision.RewriteArguments != "" || (observed != nil) != test.failure {
			t.Fatalf("exit %d admitted decision = %+v, %v", test.exitCode, decision, observed)
		}
	}
}

func TestRunner_AskEscalates(t *testing.T) {
	r := newPolicyEvaluator(&commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandAsk, Reason: "review"}}}}, nil)
	hooks := []hookdata.Hook{{Event: hookdata.PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if dec.Verdict != hookdata.CommandAsk {
		t.Fatalf("got verdict=%v, want ask", dec.Verdict)
	}
}

func TestRunner_RewriteArguments(t *testing.T) {
	r := newPolicyEvaluator(&commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow, RewriteArguments: `{"path":"safe"}`}}}}, nil)
	hooks := []hookdata.Hook{{Event: hookdata.PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "write"}})
	if dec.RewriteArguments != `{"path":"safe"}` {
		t.Fatalf("RewriteArguments = %q", dec.RewriteArguments)
	}
}

func TestRunner_NonBlockingErrorProceeds(t *testing.T) {
	var mu sync.Mutex
	var errs []string
	cmds := &commandStub{results: []commandResult{{
		Stderr:   "boom",
		ExitCode: 3,
		Err:      errors.New("exit status 3"),
	}}}
	r := newPolicyEvaluator(cmds, func(_ context.Context, _ string, err error) {
		mu.Lock()
		errs = append(errs, err.Error())
		mu.Unlock()
	})
	hooks := []hookdata.Hook{{Event: hookdata.PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if dec.Verdict == hookdata.CommandDeny {
		t.Error("a non-2 exit must NOT block (broken hook can't brick the agent)")
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "boom") {
		t.Fatalf("onError = %v, want one error mentioning boom", errs)
	}
}

func TestRunnerPreservesCommandFailureEvidence(t *testing.T) {
	processFailure := errors.New("process failed")
	cleanupFailure := errors.New("cleanup failed")
	for _, test := range []struct {
		name   string
		result commandResult
		causes []error
	}{
		{"stderr", commandResult{ExitCode: 3, Stderr: "diagnostic", Err: errors.Join(processFailure, cleanupFailure)}, []error{processFailure, cleanupFailure}},
		{"timeout", commandResult{TimedOut: true, Err: errors.Join(context.DeadlineExceeded, cleanupFailure)}, []error{context.DeadlineExceeded, cleanupFailure}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.result.Decision = hookdata.CommandDecision{Verdict: hookdata.CommandAllow, InjectContext: "unadmitted", RewriteArguments: `{}`}
			var observed error
			runner := newPolicyEvaluator(&commandStub{results: []commandResult{test.result}}, func(_ context.Context, _ string, err error) { observed = err })
			decision := runner.Run(t.Context(), []hookdata.Hook{{Event: hookdata.PreToolUse, Command: "hook"}}, hookdata.Input{
				Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"},
			})
			for _, cause := range test.causes {
				if !errors.Is(observed, cause) {
					t.Fatalf("observed failure = %v, want retained cause %v", observed, cause)
				}
			}
			if decision.Verdict == hookdata.CommandDeny || decision.InjectContext != "" || decision.RewriteArguments != "" {
				t.Fatalf("failed command advanced decision material: %+v", decision)
			}
		})
	}
}

func TestRunner_InvalidVerdictIsObservableAndIgnored(t *testing.T) {
	var got error
	r := newPolicyEvaluator(
		&commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: "future"}}}},
		func(_ context.Context, _ string, err error) { got = err },
	)
	decision := r.Run(ctxBG(), []hookdata.Hook{{
		Event: hookdata.PreToolUse, Command: "hook",
	}}, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if decision.Verdict != hookdata.CommandAllow || decision.InjectContext != "" || decision.RewriteArguments != "" {
		t.Fatalf("invalid verdict changed decision: %+v", decision)
	}
	if got == nil || !strings.Contains(got.Error(), "invalid command verdict") {
		t.Fatalf("onError = %v, want invalid verdict error", got)
	}
}

func TestRunner_TimeoutIsNonBlocking(t *testing.T) {
	var got error
	r := newPolicyEvaluator(&commandStub{results: []commandResult{{TimedOut: true}}}, func(_ context.Context, _ string, err error) { got = err })
	hooks := []hookdata.Hook{{Event: hookdata.PreToolUse, Command: "hook", TimeoutMillis: 40}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if dec.Verdict == hookdata.CommandDeny {
		t.Error("a timed-out hook must not block")
	}
	if got == nil || !strings.Contains(got.Error(), "timed out") {
		t.Fatalf("onError = %v, want timeout", got)
	}
}

func TestRunner_MatcherGatesByToolName(t *testing.T) {
	cmds := &commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandDeny, Reason: "x"}}}}
	r := newPolicyEvaluator(cmds, nil)
	hooks := []hookdata.Hook{{Event: hookdata.PreToolUse, Matcher: "shell", Command: "hook"}}

	denied := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if denied.Verdict != hookdata.CommandDeny {
		t.Error("matcher shell should fire for shell")
	}
	passed := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "read"}})
	if passed.Verdict == hookdata.CommandDeny {
		t.Error("matcher shell must NOT fire for read")
	}
	if cmds.calls() != 1 {
		t.Fatalf("commands called %d times, want only the matching hook", cmds.calls())
	}
}

func TestRunner_FirstBlockWins_ContextConcatenated(t *testing.T) {
	r := newPolicyEvaluator(&commandStub{results: []commandResult{
		{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandDeny, Reason: "first"}},
		{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandDeny, Reason: "second"}},
	}}, nil)
	hooks := []hookdata.Hook{
		{Event: hookdata.PostToolUse, Inject: "ctx-a"},
		{Event: hookdata.PostToolUse, Command: "hook"},
		{Event: hookdata.PostToolUse, Command: "hook"},
		{Event: hookdata.PostToolUse, Inject: "ctx-b"},
	}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PostToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if dec.Reason != "first" {
		t.Errorf("Reason = %q, want first-block-wins", dec.Reason)
	}
	if dec.InjectContext != "ctx-a\nctx-b" {
		t.Errorf("InjectContext = %q, want both concatenated", dec.InjectContext)
	}
}

func TestRunner_WrongEventDoesNotFire(t *testing.T) {
	cmds := &commandStub{results: []commandResult{{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandDeny}}}}
	r := newPolicyEvaluator(cmds, nil)
	hooks := []hookdata.Hook{{Event: hookdata.Stop, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdata.Input{Event: hookdata.PreToolUse, Tool: &hookdata.ToolInput{Name: "shell"}})
	if dec.Verdict == hookdata.CommandDeny {
		t.Error("a Stop hook must not fire on PreToolUse")
	}
	if cmds.calls() != 0 {
		t.Fatalf("commands called %d times, want none", cmds.calls())
	}
}
func TestFoldDecidesTheSameCallWhateverOrderHooksFire(t *testing.T) {
	deny := func(d *eventAdvice) { d.Fold(hookdata.CommandDeny, "blocked by policy", "", "") }
	ask := func(d *eventAdvice) { d.Fold(hookdata.CommandAsk, "please review", "", "") }
	rewrite := func(d *eventAdvice) { d.Fold(hookdata.CommandAllow, "", "", `{"path":"/tmp"}`) }

	for _, test := range []struct {
		name  string
		order []func(*eventAdvice)
	}{
		{name: "deny first", order: []func(*eventAdvice){deny, ask, rewrite}},
		{name: "ask then deny", order: []func(*eventAdvice){ask, deny}},
		{name: "rewrite then deny", order: []func(*eventAdvice){rewrite, deny}},
		{name: "ask, rewrite, then deny", order: []func(*eventAdvice){ask, rewrite, deny}},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision := eventAdvice{CommandDecision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow}}
			for _, fold := range test.order {
				fold(&decision)
			}
			if decision.Verdict != hookdata.CommandDeny || decision.Reason != "blocked by policy" {
				t.Fatalf("decision = %+v, want a denial stating its reason", decision)
			}
			if decision.RewriteArguments != "" {
				t.Fatalf("denied decision still rewrites: %+v", decision)
			}
		})
	}
}
