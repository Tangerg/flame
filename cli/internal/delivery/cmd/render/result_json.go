package render

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

// resultPhase orders the one result object: it has nothing to emit until a
// run starts, and nothing more to fold once closed.
type resultPhase uint8

const (
	resultPending resultPhase = iota
	resultStarted
	resultClosed
)

// ResultJSON folds a streamed run into one final JSON object. It retains only
// assistant prose and terminal metadata; callers that need every event use [NDJSON].
type ResultJSON struct {
	out   io.Writer
	err   error
	phase resultPhase
	scope runScope
	frame resultFrame
	prose assistantProse
}

type assistantProse struct {
	blocks []assistantProseBlock
	index  map[string]int
}

type assistantProseBlock struct {
	text conversation.StreamedText
}

func (a *assistantProse) reset() {
	a.blocks = nil
	a.index = make(map[string]int)
}

func (a *assistantProse) begin(block conversation.Block) {
	a.ensureIndex()
	if at, exists := a.index[block.ID]; exists {
		a.blocks[at].text = conversation.NewStreamedText(block.Text)
		return
	}
	a.index[block.ID] = len(a.blocks)
	a.blocks = append(a.blocks, assistantProseBlock{text: conversation.NewStreamedText(block.Text)})
}

func (a *assistantProse) delta(delta conversation.BlockDelta) error {
	a.ensureIndex()
	at, exists := a.index[delta.BlockID]
	if !exists {
		return nil
	}
	return a.blocks[at].text.Apply(delta)
}

func (a *assistantProse) complete(block conversation.Block) {
	a.ensureIndex()
	at, exists := a.index[block.ID]
	if !exists {
		a.index[block.ID] = len(a.blocks)
		a.blocks = append(a.blocks, assistantProseBlock{})
		at = len(a.blocks) - 1
	}
	a.blocks[at].text = conversation.NewStreamedText(block.Text)
}

func (a *assistantProse) text() string {
	parts := make([]string, 0, len(a.blocks))
	for _, block := range a.blocks {
		if text := block.text.String(); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (a *assistantProse) ensureIndex() {
	if a.index == nil {
		a.index = make(map[string]int)
	}
}

type resultFrame struct {
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	RunID      string          `json:"runId"`
	SessionID  string          `json:"sessionId"`
	Text       string          `json:"text,omitzero"`
	Images     []imageFrame    `json:"images,omitzero"`
	Options    *runOptionsJSON `json:"options,omitzero"`
	Interrupts []interruptJSON `json:"interrupts,omitzero"`
	Outcome    *outcomeJSON    `json:"outcome,omitzero"`
	Usage      *usageJSON      `json:"usage,omitzero"`
}

// NewResultJSON builds a renderer that emits at most one JSON result from Close.
func NewResultJSON(w io.Writer) *ResultJSON {
	return &ResultJSON{out: w}
}

// Begin records the accepted run before its first subscription opens, so a
// transport failure can still produce a useful incomplete result.
func (r *ResultJSON) Begin(run conversation.Run, options prompt.RunOptions) error {
	if r.err != nil {
		return r.err
	}
	if r.phase == resultClosed {
		r.err = errors.New("begin result after close")
		return r.err
	}
	if err := run.Validate(); err != nil {
		r.err = fmt.Errorf("begin result: %w", err)
		return r.err
	}
	if err := r.scope.bind(run); err != nil {
		r.err = fmt.Errorf("begin result: %w", err)
		return r.err
	}
	r.phase = resultStarted
	r.frame = resultFrame{
		Type: "result", Status: string(protocol.RunStatusRunning), RunID: run.ID,
		SessionID: run.SessionID, Options: encodeRunOptions(options),
	}
	return nil
}

// Render folds one validated event into the final result.
func (r *ResultJSON) Render(envelope conversation.RunEvent) error {
	if r.err != nil {
		return r.err
	}
	if r.phase == resultClosed {
		r.err = errors.New("render result after close")
		return r.err
	}
	if err := conversation.ValidateEvent(envelope.Event); err != nil {
		r.err = fmt.Errorf("render result event: %w", err)
		return r.err
	}
	if err := r.scope.accept(envelope); err != nil {
		r.err = fmt.Errorf("render result event: %w", err)
		return r.err
	}
	if r.frame.RunID == "" {
		r.frame.RunID = r.scope.rootID
	}
	r.fold(envelope)
	return r.err
}

// Reconcile replaces the folded result with durable cold-read values after the
// runtime reports that the live segment can no longer be replayed.
func (r *ResultJSON) Reconcile(snapshot conversation.SessionSnapshot) error {
	if r.err != nil {
		return r.err
	}
	if r.phase == resultClosed {
		r.err = errors.New("reconcile result after close")
		return r.err
	}
	if err := snapshot.Validate(); err != nil {
		r.err = fmt.Errorf("reconcile result snapshot: %w", err)
		return r.err
	}
	r.phase = resultStarted
	r.prose.reset()
	r.frame.Images = nil
	target, err := resolveSnapshotRun(snapshot, r.frame.RunID)
	if err != nil {
		r.err = fmt.Errorf("render result snapshot: %w", err)
		return r.err
	}
	targetRunID := target.ID
	if err := r.scope.restore(snapshot, targetRunID); err != nil {
		r.err = fmt.Errorf("render result snapshot: %w", err)
		return r.err
	}
	for _, block := range snapshot.Transcript {
		if block.RunID == targetRunID && block.Status != conversation.BlockStatusRunning && block.Kind == conversation.BlockAssistant {
			r.prose.complete(block)
			r.appendImages(block.Images)
		}
	}
	r.frame.RunID = targetRunID
	r.frame.SessionID = snapshot.Session.ID
	r.frame.Status = string(protocol.RunStatusRunning)
	r.frame.Interrupts = nil
	r.frame.Outcome = nil
	r.frame.Usage = nil
	switch target.Status {
	case protocol.RunStatusWaiting:
		r.frame.Status = string(protocol.RunStatusWaiting)
		r.frame.Interrupts = encodeInterrupts(snapshot.Interrupts)
		r.frame.Usage = encodeUsage(target.Usage)
	case protocol.RunStatusFinished:
		r.frame.Status = string(protocol.RunStatusFinished)
		finished := encodeFinishedFrame(conversation.RunFinished{Outcome: target.Outcome, Usage: target.Usage})
		r.frame.Outcome, r.frame.Usage = finished.Outcome, finished.Usage
	case protocol.RunStatusRunning:
	}
	return nil
}

func (r *ResultJSON) fold(envelope conversation.RunEvent) {
	switch event := envelope.Event.(type) {
	case conversation.SegmentStarted:
		if !event.Run.Lineage.IsRoot() {
			return
		}
		if r.phase == resultPending {
			r.phase = resultStarted
			r.frame = resultFrame{Type: "result", Status: string(protocol.RunStatusRunning)}
		}
		r.frame.RunID, r.frame.SessionID = event.Run.ID, event.Run.SessionID
		r.frame.Status = string(protocol.RunStatusRunning)
		r.frame.Interrupts = nil
		if r.frame.RunID == "" {
			r.frame.RunID = envelope.RunID
		}
	case conversation.BlockStarted:
		if r.scope.isRoot(envelope.RunID) {
			r.begin(event.Block)
		}
	case conversation.BlockDelta:
		if r.scope.isRoot(envelope.RunID) {
			if err := r.prose.delta(event); err != nil {
				r.err = fmt.Errorf("fold result delta %s: %w", event.BlockID, err)
			}
		}
	case conversation.RunProgress:
		if r.scope.isRoot(envelope.RunID) && event.Usage != nil {
			r.frame.Usage = encodeUsage(*event.Usage)
		}
	case conversation.BlockCompleted:
		if r.scope.isRoot(envelope.RunID) {
			r.complete(event.Block)
		}
	case conversation.RunInterrupted:
		r.frame.Interrupts = append(r.frame.Interrupts, encodeInterrupts(event.Interrupts)...)
		if r.scope.isRoot(envelope.RunID) {
			r.frame.Status = string(protocol.RunStatusWaiting)
			r.frame.Usage = encodeUsage(event.Usage)
		}
	case conversation.RunSuspended:
		if r.scope.isRoot(envelope.RunID) {
			r.frame.Status = string(protocol.RunStatusWaiting)
			r.frame.Usage = encodeUsage(event.Usage)
		}
	case conversation.RunFinished:
		if r.scope.isRoot(envelope.RunID) {
			r.frame.Status = string(protocol.RunStatusFinished)
			r.frame.Interrupts = nil
			finished := encodeFinishedFrame(event)
			r.frame.Outcome, r.frame.Usage = finished.Outcome, finished.Usage
		}
	case conversation.PlanChanged, conversation.ToolArgumentsDelta, conversation.CustomEvent:
		// A final result intentionally omits incremental plan state.
	}
}

func (r *ResultJSON) begin(block conversation.Block) {
	if block.Kind != conversation.BlockAssistant {
		return
	}
	r.prose.begin(block)
}

func (r *ResultJSON) complete(block conversation.Block) {
	if block.Kind != conversation.BlockAssistant {
		return
	}
	r.prose.complete(block)
	r.appendImages(block.Images)
}

func (r *ResultJSON) appendImages(images []conversation.InlineImage) {
	for _, image := range images {
		r.frame.Images = append(r.frame.Images, imageFrame{
			ID: image.ID, Name: image.Name, MIMEType: image.MIMEType,
			Data: image.Data, Size: int64(len(image.Data)),
		})
	}
}

// Close emits one object after at least a run.started event. It is idempotent;
// failures before a run starts leave stdout empty.
func (r *ResultJSON) Close() error {
	if r.phase == resultClosed {
		return r.err
	}
	started := r.phase == resultStarted
	r.phase = resultClosed
	if r.err != nil || !started {
		return r.err
	}
	r.frame.Text = r.prose.text()
	r.err = WriteJSONLine(r.out, r.frame)
	return r.err
}
