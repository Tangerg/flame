package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestSessionHeaderExposesMissingWorkspace(t *testing.T) {
	t.Parallel()

	header := newSessionHeader(kit.Dark(), kit.Unicode(), conversation.Session{
		Title: "History", Workspace: workspace.Workspace{
			Path: "/gone/work", ProjectRoot: "/gone", Availability: protocol.WorkspaceMissing,
		},
	})
	got := drawStatic(t, header, 72, 2)
	for _, want := range []string{"/gone/work", "missing", "History"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing workspace header does not contain %q:\n%s", want, got)
		}
	}
}

func TestGoalUsageLabelDoesNotPresentPartialCostAsTotal(t *testing.T) {
	if got := goalUsageLabel(protocol.GoalUsage{Runs: 2, Steps: 3}); !strings.Contains(got, "cost unavailable") {
		t.Fatalf("unknown Goal cost label = %q", got)
	}
}

func TestStatusProgressIncludesRuntimeActivityStepAndContext(t *testing.T) {
	status := newStatusView(kit.Dark(), kit.Unicode())
	step, contextTokens := 7, int64(12_345)
	status.progress(conversation.RunProgress{Step: &step, ContextTokens: &contextTokens, Activity: &protocol.Activity{Type: protocol.ActivityCallingModel}})
	if status.phase != statusActive || status.doing != "calling model · step 7" || status.contextTokens != contextTokens {
		t.Fatalf("progress status = phase %d, doing %q", status.phase, status.doing)
	}
	status.active("using shell")
	if got := drawStatic(t, status, 72, 1); !strings.Contains(got, "using shell") || !strings.Contains(got, "ctx 12,345") {
		t.Fatalf("sparse activity lost the latest context footprint:\n%s", got)
	}
	status.settled(conversation.Run{
		ContextTokens: contextTokens,
		Outcome:       conversation.Outcome{Status: protocol.OutcomeCompleted},
		Usage:         conversation.Usage{InputTokens: 20, OutputTokens: 4},
	})
	if got := drawStatic(t, status, 72, 1); !strings.Contains(got, "complete") ||
		!strings.Contains(got, "ctx 12,345") || !strings.Contains(got, "↑20") {
		t.Fatalf("settled status lost the final Run footprint:\n%s", got)
	}
	status.beginRun("starting")
	if status.contextTokens != 0 || status.usage.InputTokens != 0 {
		t.Fatalf("new Run retained the prior footprint: context %d, usage %+v", status.contextTokens, status.usage)
	}
}

func TestSettledStatusIncludesRunRecoveryMetadata(t *testing.T) {
	status := newStatusView(kit.Dark(), kit.Unicode())
	status.settled(conversation.Run{Outcome: conversation.Outcome{
		Status: protocol.OutcomeFailed,
		Problem: &protocol.ProblemData{
			Type: "rate_limited", Detail: "quota exhausted", RetryAfterSeconds: 12,
		},
	}})
	for _, want := range []string{"quota exhausted", "retry after 12s"} {
		if !strings.Contains(status.doing, want) {
			t.Fatalf("settled status omitted %q: %q", want, status.doing)
		}
	}
}

func TestClientFailureDoesNotInventARunOutcome(t *testing.T) {
	status := newStatusView(kit.Dark(), kit.Unicode())
	status.fail("event projection rejected", true)
	if status.phase != statusActive || status.outcome.Status != "" ||
		status.doing != "client failed: event projection rejected" {
		t.Fatalf("client failure while work continues = %+v", status)
	}
	status.fail("event projection rejected", false)
	if status.phase != statusFailed || status.outcome.Status != "" {
		t.Fatalf("client failure after work stopped = %+v", status)
	}
	status.active("reconnected")
	if status.phase != statusActive {
		t.Fatalf("active status retained local failure: %+v", status)
	}
}

func TestBusyStatusExposesLiveDelegatedWorkWithoutHidingProgress(t *testing.T) {
	status := newStatusView(kit.Dark(), kit.Unicode())
	status.active("inspecting architecture")
	status.setRunningDescendants(2)

	wide := drawStatic(t, status, 72, 1)
	for _, want := range []string{"inspecting architecture", "2 subagents active"} {
		if !strings.Contains(wide, want) {
			t.Errorf("busy status does not contain %q:\n%s", want, wide)
		}
	}

	narrow := drawStatic(t, status, 18, 1)
	if strings.TrimSpace(narrow) == "" || strings.Contains(narrow, "subagents active") {
		t.Fatalf("narrow busy status did not preserve primary progress:\n%s", narrow)
	}
}

func TestStatusKeepsAnUnresolvedWorkbenchProblemAboveRunProgress(t *testing.T) {
	status := newStatusView(kit.Dark(), kit.Unicode())
	status.setProblem("workbench: state could not be committed")
	status.active("working")
	if got := drawStatic(t, status, 72, 1); !strings.Contains(got, "state could not be committed") || strings.Contains(got, "working") {
		t.Fatalf("workbench problem did not dominate progress:\n%s", got)
	}
	status.setProblem("")
	if got := drawStatic(t, status, 72, 1); !strings.Contains(got, "working") {
		t.Fatalf("cleared workbench problem did not restore progress:\n%s", got)
	}
}

func TestStatusProjectsMultilineTextIntoItsSingleRow(t *testing.T) {
	status := newStatusView(kit.Dark(), kit.Unicode())
	status.active("reading\nprovider\tresponse\x00")
	if got := drawStatic(t, status, 72, 1); !strings.Contains(got, "reading provider response") {
		t.Fatalf("multiline progress was not projected as one readable row:\n%s", got)
	}

	status.setProblem("workbench\nstate\tfailed\x00")
	if got := drawStatic(t, status, 72, 1); !strings.Contains(got, "workbench state failed") {
		t.Fatalf("multiline problem was not projected as one readable row:\n%s", got)
	}
}

func TestShellUsesTwoRowChromeOnTinyTerminals(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testTranscriptView(t)
	transcript.Append(&kit.Entry{Theme: theme, Label: "flame", Body: "VISIBLE_TRANSCRIPT"})
	header := newSessionHeader(theme, glyphs, conversation.Session{Title: "Hidden title", Workspace: workspace.Workspace{
		Path: "/hidden/workspace", ProjectRoot: "/hidden/workspace", Availability: protocol.WorkspaceAvailable,
	}})
	activity := newActivityView(theme, glyphs)
	activity.Set([]protocol.PlanStep{{Description: "HIDDEN_PLAN", Status: protocol.PlanStatusInProgress}})
	composer := kit.NewComposer(kit.ComposerConfig{Theme: theme, Glyphs: glyphs, Prompt: glyphs.Marker + " ", MaxRows: 6})
	composer.Controller().SetKeys(bindings.editor)
	composer.Controller().SetText("TINY_DRAFT")
	authoredPrompt := newPromptView(theme, glyphs, bindings.editor, composer, defaultRunOptions(t))
	shell := newShellView(header, transcript, activity, newQueueView(theme, glyphs), newStatusView(theme, glyphs), authoredPrompt)

	tiny := drawRoot(t, shell, 20, minimalShellHeight-1)
	for _, want := range []string{"VISIBLE", "TINY_DRAFT", "rea"} {
		if !strings.Contains(tiny, want) {
			t.Errorf("tiny shell does not contain %q:\n%s", want, tiny)
		}
	}
	for _, hidden := range []string{"/hidden/workspace", "HIDDEN_PLAN", "shift+enter"} {
		if strings.Contains(tiny, hidden) {
			t.Errorf("tiny shell retained %q:\n%s", hidden, tiny)
		}
	}

	normal := drawRoot(t, shell, 96, 28)
	for _, want := range []string{"/hidden/workspace", "HIDDEN_PLAN", "TINY_DRAFT", "shift+enter"} {
		if !strings.Contains(normal, want) {
			t.Errorf("restored shell does not contain %q:\n%s", want, normal)
		}
	}
}

func TestResponsiveShellPreservesTranscriptFocusAndDraft(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testTranscriptView(t)
	appendTestTool(transcript, "focus", "detail")
	composer := kit.NewComposer(kit.ComposerConfig{Theme: theme, Glyphs: glyphs, Prompt: glyphs.Marker + " ", MaxRows: 6})
	composer.Controller().SetKeys(bindings.editor)
	composer.Controller().SetText("PRESERVED_DRAFT")
	authoredPrompt := newPromptView(theme, glyphs, bindings.editor, composer, defaultRunOptions(t))
	shell := newShellView(
		newSessionHeader(theme, glyphs, conversation.Session{}), transcript,
		newActivityView(theme, glyphs), newQueueView(theme, glyphs),
		newStatusView(theme, glyphs), authoredPrompt,
	)
	root := newSceneTestRoot(t, shell.Element())
	root.Draw(grid.NewSurface(96, 28).View())
	if !shell.FocusPrompt() {
		t.Fatal("cannot focus prompt")
	}
	if !root.Handle(input.Key{Code: input.Tab}) || !shell.TranscriptFocused() {
		t.Fatal("test could not focus the transcript")
	}

	root.Draw(grid.NewSurface(20, minimalShellHeight-1).View())
	if !shell.TranscriptFocused() {
		t.Fatal("entering compact layout moved focus away from the transcript")
	}
	root.Draw(grid.NewSurface(96, 28).View())
	if !shell.TranscriptFocused() {
		t.Fatal("leaving compact layout moved focus away from the transcript")
	}
	if got := composer.Controller().Text(); got != "PRESERVED_DRAFT" {
		t.Fatalf("resize changed draft to %q", got)
	}
}

func TestShellMovesFocusBetweenPromptAndTranscript(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testTranscriptView(t)
	appendTestTool(transcript, "focus", "detail")
	composer := kit.NewComposer(kit.ComposerConfig{Theme: theme, Glyphs: glyphs, Prompt: glyphs.Marker + " ", MaxRows: 6})
	authoredPrompt := newPromptView(theme, glyphs, bindings.editor, composer, defaultRunOptions(t))
	shell := newShellView(
		newSessionHeader(theme, glyphs, conversation.Session{}), transcript,
		newActivityView(theme, glyphs), newQueueView(theme, glyphs),
		newStatusView(theme, glyphs), authoredPrompt,
	)
	authoredPrompt.SetTranscriptKeys(transcript.Keys())
	transcript.OnFocusChange(authoredPrompt.SetTranscriptFocused)
	transcript.OnSelection(authoredPrompt.SetTranscriptSelection)

	root := newSceneTestRoot(t, shell.Element())
	root.Draw(grid.NewSurface(96, 28).View())
	if !shell.FocusPrompt() {
		t.Fatal("cannot focus prompt")
	}
	if !shell.PromptFocused() {
		t.Fatal("new shell did not start with prompt focus")
	}
	if !root.Handle(input.Key{Code: input.Tab}) || !shell.TranscriptFocused() {
		t.Fatal("Tab did not move focus to the transcript")
	}
	surface := grid.NewSurface(96, 20)
	root.Draw(surface.View())
	focused := strings.Join(surface.Rows(), "\n")
	for _, want := range []string{"select prev", "select next", "toggle details", "prompt"} {
		if !strings.Contains(focused, want) {
			t.Errorf("transcript help does not contain %q:\n%s", want, focused)
		}
	}
	if !root.Handle(input.Key{Code: input.Character, Rune: ' '}) || !shell.PromptFocused() {
		t.Fatal("Space did not return focus to the prompt")
	}
}
