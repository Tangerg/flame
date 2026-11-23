package runtimefixture

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	interruptInputTokens     = 12_800
	interruptOutputTokens    = 684
	interruptCacheReadTokens = 9_600
	interruptCostUSD         = 0.0264
	interruptDuration        = 9 * time.Second

	approvedInputTokens     = 18_422
	approvedOutputTokens    = 1_163
	approvedCacheReadTokens = 12_800
	approvedCostUSD         = 0.0412
	approvedDuration        = 21 * time.Second

	deniedInputTokens     = 14_180
	deniedOutputTokens    = 742
	deniedCacheReadTokens = 12_800
	deniedCostUSD         = 0.0291
	deniedDuration        = 14 * time.Second

	cacheSessionRevision   = 7
	renameSessionRevision  = 3
	releaseSessionRevision = 12
)

const (
	scriptedRunID = "fixture"
	demoSessionID = "ses_demo_1"
	demoTestFile  = "internal/store/cache_test.go"
)

type Step struct {
	Delay  time.Duration
	Event  conversation.Event
	Finish *Finish
	plan   *planReplacementAction
}

type Finish struct {
	Outcome conversation.Outcome
	Usage   conversation.Usage
}

type planReplacementAction struct {
	steps []protocol.PlanStep
	err   error
}

func finishStep(finish Finish) Step {
	return Step{Delay: beat, Finish: &finish}
}

func Emit(delay time.Duration, event conversation.Event) Step {
	return Step{Delay: delay, Event: event}
}

func ReplacePlan(delay time.Duration, steps []protocol.PlanStep) Step {
	cloned := slices.Clone(steps)
	var err error
	for index, step := range cloned {
		if validationErr := protocol.ValidateWireTree(step); validationErr != nil {
			err = fmt.Errorf("plan step %d: %w", index+1, validationErr)
			break
		}
	}
	return Step{Delay: delay, plan: &planReplacementAction{steps: cloned, err: err}}
}

type Script struct {
	Prelude        []Step
	Interrupts     []conversation.Interrupt
	InterruptUsage conversation.Usage
	Continue       func([]conversation.InterruptAnswer) []Step
}

func buildScriptSafely(build func(string) Script, authoredPrompt string) (script Script, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("script builder panicked: %v", recovered)
		}
	}()
	script = cloneScript(build(authoredPrompt))
	if err := script.validate(); err != nil {
		return Script{}, err
	}
	return script, nil
}

func (s Script) validate() error {
	interrupted := s.interrupts()
	if interrupted {
		interrupts := conversation.CloneInterrupts(s.Interrupts)
		for i, interrupt := range interrupts {
			interrupts[i] = bindInterruptToRun(interrupt, scriptedRunID)
		}
		if err := conversation.ValidateInterrupts(interrupts); err != nil {
			return err
		}
	} else if s.Continue != nil {
		return errors.New("script without an interrupt has a continuation")
	}
	return validateSteps(s.Prelude, !interrupted)
}

func continueSafely(script Script, answers []conversation.InterruptAnswer) (steps []Step, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("script continuation panicked: %v", recovered)
		}
	}()
	if script.Continue == nil {
		return nil, errors.New("script interrupt has no continuation")
	}
	steps = cloneSteps(script.Continue(cloneAnswers(answers)))
	if err := validateSteps(steps, true); err != nil {
		return nil, err
	}
	return steps, nil
}

func validateSteps(steps []Step, requireFinish bool) error {
	finished := false
	for i, step := range steps {
		finishes, err := step.validate(i == len(steps)-1)
		if err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		finished = finished || finishes
	}
	if requireFinish && !finished {
		return errors.New("script path does not finish the run")
	}
	return nil
}

func (s Step) validate(last bool) (bool, error) {
	if s.Delay < 0 {
		return false, errors.New("negative delay")
	}
	actions := 0
	for _, present := range []bool{s.Event != nil, s.Finish != nil, s.plan != nil} {
		if present {
			actions++
		}
	}
	if actions != 1 {
		return false, errors.New("must have exactly one action")
	}
	switch {
	case s.Event != nil:
		switch s.Event.(type) {
		case conversation.SegmentStarted, conversation.SegmentFinished, conversation.PlanChanged:
			return false, errors.New("contains a runtime-owned event")
		}
		return false, conversation.ValidateEvent(ownedBlockEvent(scriptedRunID, s.Event))
	case s.Finish != nil:
		if !last {
			return false, errors.New("finishes before the script ends")
		}
		return true, nil
	default:
		return false, s.plan.err
	}
}

func cloneScript(script Script) Script {
	script.Prelude = cloneSteps(script.Prelude)
	script.Interrupts = conversation.CloneInterrupts(script.Interrupts)
	script.InterruptUsage = script.InterruptUsage.Clone()
	return script
}

func completedBlockStatus(block conversation.Block) conversation.BlockStatus {
	if block.Kind == conversation.BlockError || block.Tool != nil && block.Tool.Status.Failed() {
		return conversation.BlockStatusIncomplete
	}
	return conversation.BlockStatusCompleted
}

func cloneSteps(steps []Step) []Step {
	cloned := slices.Clone(steps)
	for i := range cloned {
		cloned[i].Event = conversation.CloneEvent(cloned[i].Event)
		if cloned[i].Finish != nil {
			finish := Finish{Outcome: cloned[i].Finish.Outcome.Clone(), Usage: cloned[i].Finish.Usage.Clone()}
			cloned[i].Finish = &finish
		}
		if cloned[i].plan != nil {
			plan := *cloned[i].plan
			plan.steps = slices.Clone(plan.steps)
			cloned[i].plan = &plan
		}
	}
	return cloned
}

func namespaceScript(script Script, runID string) Script {
	originalContinue := script.Continue
	script.Prelude = namespaceSteps(script.Prelude, runID)
	for i, interrupt := range script.Interrupts {
		script.Interrupts[i] = namespaceInterrupt(interrupt, runID)
	}
	if originalContinue != nil {
		script.Continue = func(answers []conversation.InterruptAnswer) []Step {
			local := cloneAnswers(answers)
			for i := range local {
				local[i].ItemID = strings.TrimPrefix(local[i].ItemID, runID+":")
			}
			return namespaceSteps(originalContinue(local), runID)
		}
	}
	return script
}

func namespaceSteps(steps []Step, runID string) []Step {
	out := cloneSteps(steps)
	for i, step := range out {
		switch event := step.Event.(type) {
		case conversation.BlockStarted:
			event.Block.ID = runID + ":" + event.Block.ID
			event.Block.RunID = runID
			out[i].Event = event
		case conversation.BlockDelta:
			event.BlockID = runID + ":" + event.BlockID
			out[i].Event = event
		case conversation.BlockCompleted:
			event.Block.ID = runID + ":" + event.Block.ID
			event.Block.RunID = runID
			out[i].Event = event
		}
	}
	return out
}

func namespaceInterrupt(interrupt conversation.Interrupt, runID string) conversation.Interrupt {
	interrupt = bindInterruptToRun(interrupt, runID)
	switch item := interrupt.(type) {
	case conversation.Approval:
		item.ItemID = runID + ":" + item.ItemID
		return item
	case conversation.Question:
		item.ItemID = runID + ":" + item.ItemID
		return item
	default:
		return nil
	}
}

func bindInterruptToRun(interrupt conversation.Interrupt, runID string) conversation.Interrupt {
	switch item := interrupt.(type) {
	case conversation.Approval:
		item.RunID = runID
		return item
	case conversation.Question:
		item.RunID = runID
		return item
	default:
		return nil
	}
}

func (s Script) interrupts() bool { return len(s.Interrupts) != 0 }

const (
	tick = 22 * time.Millisecond
	beat = 260 * time.Millisecond

	outputChunkDelay      = 3 * tick
	silentCompletionDelay = 3 * beat
)

func DefaultScript(_ string) Script {
	return newDefaultScenario().script()
}

type defaultScenario struct {
	reasoning   string
	explanation string
	summary     string
	declined    string
	patch       string
	testOutput  string
}

func newDefaultScenario() defaultScenario {
	return defaultScenario{
		reasoning: "The failure is intermittent, roughly one run in five, which points at timing rather than logic. " +
			"Let me run the test several times and read what the failing run actually reports.",
		explanation: "`TestCacheExpiry` waits a fixed **50ms** for the janitor goroutine to evict, then asserts the entry is gone.\n\n" +
			"On a loaded machine the janitor has not always woken by then, so the assertion reads a value that is about to be\n" +
			"removed. The sleep is the bug — the test needs to wait for the eviction, not for a duration.\n",
		summary: "Replaced the sleep with a wait on the janitor's eviction signal. Ran the test 50 times: no failures.\n",
		declined: "Left the file alone. If you would rather fix it differently, the janitor already exposes a channel that\n" +
			"closes after each sweep — waiting on that is the smallest change that removes the timing assumption.\n",
		patch: strings.Join([]string{
			"--- a/" + demoTestFile,
			"+++ b/" + demoTestFile,
			"@@ -18,8 +18,8 @@ func TestCacheExpiry(t *testing.T) {",
			" \tc := New(WithTTL(10 * time.Millisecond))",
			" \tc.Set(\"k\", \"v\")",
			" ",
			"-\ttime.Sleep(50 * time.Millisecond)",
			"-",
			"+\t<-c.swept()",
			"+",
			" \tif _, ok := c.Get(\"k\"); ok {",
			" \t\tt.Fatal(\"expected k to have expired\")",
			" \t}",
			"",
		}, "\n"),
		testOutput: strings.Join([]string{
			"--- FAIL: TestCacheExpiry (0.05s)",
			"    cache_test.go:24: expected k to have expired",
			"FAIL",
			"FAIL\tgithub.com/example/store\t0.412s",
			"ok  \tgithub.com/example/store\t0.409s",
			"ok  \tgithub.com/example/store\t0.407s",
			"--- FAIL: TestCacheExpiry (0.05s)",
			"    cache_test.go:24: expected k to have expired",
			"FAIL",
			"ok  \tgithub.com/example/store\t0.410s",
		}, "\n"),
	}
}

func (d defaultScenario) prelude() []Step {
	prelude := make([]Step, 0, 32)
	prelude = append(prelude, ReplacePlan(beat, []protocol.PlanStep{
		{ID: "1", Description: "Reproduce the flake", Status: protocol.PlanStatusInProgress},
		{ID: "2", Description: "Find what the test is really waiting for", Status: protocol.PlanStatusPending},
		{ID: "3", Description: "Replace the sleep and re-run", Status: protocol.PlanStatusPending},
	}))
	prelude = append(prelude, stream("rsn_1", conversation.BlockReasoning, d.reasoning)...)
	prelude = append(prelude, shellTool("tool_1",
		"go test ./internal/store -run TestCacheExpiry -count=5",
		d.testOutput, 3*time.Second+412*time.Millisecond)...)
	prelude = append(prelude, ReplacePlan(beat, []protocol.PlanStep{
		{ID: "1", Description: "Reproduce the flake", Status: protocol.PlanStatusCompleted},
		{ID: "2", Description: "Find what the test is really waiting for", Status: protocol.PlanStatusCompleted},
		{ID: "3", Description: "Replace the sleep and re-run", Status: protocol.PlanStatusInProgress},
	}))
	return append(prelude, stream("msg_1", conversation.BlockAssistant, d.explanation)...)
}

func (d defaultScenario) patchArguments() []byte {
	encoded, err := json.Marshal(struct {
		Patch string `json:"patch"`
	}{d.patch})
	if err != nil {
		panic(err)
	}
	return encoded
}

func (d defaultScenario) approved() []Step {
	approved := shellTool("tool_3",
		"go test ./internal/store -run TestCacheExpiry -count=50",
		"ok  \tgithub.com/example/store\t2.104s", 2*time.Second+104*time.Millisecond)
	approved = append(approved, stream("msg_2", conversation.BlockAssistant, d.summary)...)
	approved = append(approved, finishStep(Finish{
		Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted},
		Usage: conversation.Usage{
			InputTokens: approvedInputTokens, OutputTokens: approvedOutputTokens, CacheReadTokens: approvedCacheReadTokens,
			CostUSD: new(approvedCostUSD), Duration: approvedDuration,
		},
	}))
	return approved
}

func (d defaultScenario) denied() []Step {
	denied := make([]Step, 0, 16)
	denied = append(denied, Emit(beat, conversation.BlockCompleted{Block: conversation.Block{
		ID: "note_1", Kind: conversation.BlockNotice, Text: "Edit declined — " + demoTestFile + " left unchanged.",
	}}))
	denied = append(denied, stream("msg_3", conversation.BlockAssistant, d.declined)...)
	denied = append(denied, finishStep(Finish{
		Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted},
		Usage: conversation.Usage{
			InputTokens: deniedInputTokens, OutputTokens: deniedOutputTokens, CacheReadTokens: deniedCacheReadTokens,
			CostUSD: new(deniedCostUSD), Duration: deniedDuration,
		},
	}))
	return denied
}

func (d defaultScenario) script() Script {
	approved, denied := d.approved(), d.denied()
	return Script{
		Prelude:        d.prelude(),
		InterruptUsage: conversation.Usage{InputTokens: interruptInputTokens, OutputTokens: interruptOutputTokens, CacheReadTokens: interruptCacheReadTokens, CostUSD: new(interruptCostUSD), Duration: interruptDuration},
		Interrupts: []conversation.Interrupt{conversation.Approval{
			ItemID: "tool_2",
			Title:  "edit " + demoTestFile,
			Detail: "Replace the fixed 50ms sleep with a wait on the janitor's sweep signal.",
			Tool: &conversation.ToolCall{
				Kind: builtin.CategoryEdit, Name: string(builtin.ApplyPatch), Summary: demoTestFile,
				Status: conversation.ToolRunning, Path: demoTestFile, ArgumentsJSON: d.patchArguments(),
			},
			Risk:         protocol.ApprovalRiskMedium,
			Rememberable: true,
		}},
		Continue: func(answers []conversation.InterruptAnswer) []Step {
			approval, ok := answers[0].Answer.(conversation.ApprovalAnswer)
			if ok && approval.Decision == protocol.ApprovalApprove {
				return approved
			}
			return denied
		},
	}
}

func stream(id string, kind conversation.BlockKind, text string) []Step {
	steps := []Step{Emit(beat, conversation.BlockStarted{Block: conversation.Block{ID: id, Kind: kind}})}
	for _, w := range words(text) {
		steps = append(steps, Emit(tick, conversation.BlockDelta{BlockID: id, Text: w}))
	}
	return append(steps, Emit(0, conversation.BlockCompleted{Block: conversation.Block{ID: id, Kind: kind, Text: text}}))
}

func shellTool(id, command, output string, d time.Duration) []Step {
	running := conversation.Block{ID: id, Kind: conversation.BlockTool, Tool: &conversation.ToolCall{
		Kind: builtin.CategoryShell, Name: string(builtin.Shell), Summary: command, Command: command, Status: conversation.ToolRunning,
	}}
	exitCode := 0
	done := conversation.Block{ID: id, Kind: conversation.BlockTool, Tool: &conversation.ToolCall{
		Kind: builtin.CategoryShell, Name: string(builtin.Shell), Summary: command, Command: command,
		Status: conversation.ToolOK, Output: output, Duration: d, ExitCode: &exitCode,
	}}
	steps := []Step{Emit(beat, conversation.BlockStarted{Block: running})}
	for _, chunk := range strings.SplitAfter(output, "\n") {
		if chunk != "" {
			steps = append(steps, Emit(outputChunkDelay, conversation.BlockDelta{BlockID: id, Text: chunk}))
		}
	}
	completionDelay := silentCompletionDelay
	if output != "" {
		completionDelay = beat
	}
	return append(steps, Emit(completionDelay, conversation.BlockCompleted{Block: done}))
}

func words(text string) []string {
	var out []string
	for len(text) > 0 {
		i := strings.IndexByte(text, ' ')
		if i < 0 {
			return append(out, text)
		}
		out = append(out, text[:i+1])
		text = text[i+1:]
	}
	return out
}

func demoSessions() []conversation.Session {
	now := time.Date(2026, 8, 4, 11, 30, 0, 0, time.UTC)
	return []conversation.Session{
		{ID: demoSessionID, Title: "Flaky cache expiry test", Status: protocol.SessionStatusIdle, Provider: defaultProvider, Model: defaultModel, Workspace: availableWorkspace("/tmp/demo/store"), UpdatedAt: now, Revision: cacheSessionRevision},
		{ID: "ses_demo_2", Title: "Rename the shell tool family", Status: protocol.SessionStatusIdle, Provider: defaultProvider, Model: defaultModel, Workspace: availableWorkspace("/tmp/demo/store"), UpdatedAt: now.Add(-90 * time.Minute), Revision: renameSessionRevision},
		{ID: "ses_demo_3", Title: "Draft the release notes", Status: protocol.SessionStatusIdle, Provider: defaultProvider, Model: defaultModel, Workspace: availableWorkspace("/tmp/demo/docs"), UpdatedAt: now.Add(-26 * time.Hour), Revision: releaseSessionRevision},
	}
}
