package cmd

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const maxToolOutputLines = 12

type textRenderer struct {
	w     io.Writer
	err   error
	scope outputRunScope

	streaming map[string]*plainTextStream
	pending   map[string]*pendingTextBlock
	seen      map[string]struct{}
	shown     map[string]struct{}
	settled   bool
	column    bool
}

type plainTextStream struct {
	text    conversation.StreamedText
	emitted strings.Builder
}

type pendingTextBlock struct {
	body strings.Builder
}

func newTextRenderer(w io.Writer) *textRenderer {
	return &textRenderer{
		w: w, streaming: make(map[string]*plainTextStream),
		pending: make(map[string]*pendingTextBlock), seen: make(map[string]struct{}),
		shown: make(map[string]struct{}),
	}
}

func (t *textRenderer) Begin(run conversation.Run, _ authoring.RunOptions) error {
	if t.err != nil {
		return t.err
	}
	if err := run.Validate(); err != nil {
		t.err = fmt.Errorf("begin text: %w", err)
		return t.err
	}
	if err := t.scope.bind(run); err != nil {
		t.err = fmt.Errorf("begin text: %w", err)
		return t.err
	}
	return nil
}

func (t *textRenderer) Render(envelope conversation.RunEvent) error {
	if t.err != nil {
		return t.err
	}
	if err := conversation.ValidateEvent(envelope.Event); err != nil {
		t.err = fmt.Errorf("render text event: %w", err)
		return t.err
	}
	if err := t.scope.accept(envelope); err != nil {
		t.err = fmt.Errorf("render text event: %w", err)
		return t.err
	}
	t.renderEvent(envelope)
	return t.err
}

func (t *textRenderer) renderEvent(envelope conversation.RunEvent) {
	switch event := envelope.Event.(type) {
	case conversation.SegmentStarted:
	case conversation.BlockStarted:
		t.begin(event.Block)
	case conversation.BlockDelta:
		t.delta(envelope.RunID, event)
	case conversation.ToolArgumentsDelta, conversation.RunProgress:
	case conversation.BlockCompleted:
		t.finish(event.Block)
	case conversation.PlanChanged:
		t.plan(event.Plan.State.Steps)
	case conversation.SegmentFinished:
		t.segmentFinished(envelope.RunID, event)
	default:
		t.err = fmt.Errorf("render text event: unsupported event %T", event)
	}
}

func (t *textRenderer) Close() error {
	t.endLine()
	return t.err
}

func (t *textRenderer) begin(b conversation.Block) {
	key := conversation.BlockKey(b.RunID, b.ID)
	switch b.Kind {
	case conversation.BlockAssistant:
		t.blank()
		if t.scope.isChild(b.RunID) {
			t.line("subagent · " + b.RunID)
		}
		stream := &plainTextStream{text: conversation.NewStreamedText(b.Text)}
		stream.emitted.WriteString(b.Text)
		t.streaming[key] = stream
		t.write(b.Text)
	case conversation.BlockReasoning, conversation.BlockTool, conversation.BlockUser, conversation.BlockQuestion, conversation.BlockNotice, conversation.BlockError:
		pending := &pendingTextBlock{}
		pending.body.WriteString(b.Text)
		t.pending[key] = pending
	}
}

func (t *textRenderer) delta(runID string, d conversation.BlockDelta) {
	key := conversation.BlockKey(runID, d.BlockID)
	if stream := t.streaming[key]; stream != nil {
		text, err := stream.text.Append(d)
		if err != nil {
			t.err = fmt.Errorf("render text delta %s: %w", d.BlockID, err)
			return
		}
		stream.text = text
		t.write(d.Text)
		stream.emitted.WriteString(d.Text)
		return
	}
	if pending := t.pending[key]; pending != nil {
		pending.body.WriteString(d.Text)
	}
}

func (t *textRenderer) finish(b conversation.Block) {
	key := conversation.BlockKey(b.RunID, b.ID)
	if _, duplicate := t.seen[key]; duplicate {
		return
	}
	t.seen[key] = struct{}{}
	if stream := t.streaming[key]; stream != nil {
		delete(t.streaming, key)
		authoritative := b.Text
		emitted := stream.emitted.String()
		if strings.HasPrefix(authoritative, emitted) {
			t.write(strings.TrimPrefix(authoritative, emitted))
		} else if authoritative != emitted {
			t.endLine()
			t.line("! assistant output replaced by authoritative completion")
			t.write(authoritative)
		}
		t.endLine()
		t.showImages(b.Images)
		delete(t.pending, key)
		return
	}
	text := t.completedText(b)
	delete(t.pending, key)
	t.renderCompletedBlock(b, text)
}

func (t *textRenderer) Reconcile(snapshot conversation.SessionSnapshot) error {
	if t.err != nil {
		return t.err
	}
	if err := snapshot.Validate(); err != nil {
		t.err = fmt.Errorf("render text snapshot: %w", err)
		return t.err
	}
	target, err := resolveSnapshotRun(snapshot, t.scope.rootID)
	if err != nil {
		t.err = fmt.Errorf("render text snapshot: %w", err)
		return t.err
	}
	targetRunID := target.ID
	if err := t.scope.restore(snapshot, targetRunID); err != nil {
		t.err = fmt.Errorf("render text snapshot: %w", err)
		return t.err
	}
	for _, block := range snapshot.Transcript {
		if t.scope.contains(block.RunID) {
			if block.Status == conversation.BlockStatusRunning {
				t.resume(block)
			} else {
				t.finish(block)
			}
		}
	}
	if target.Status == protocol.RunStatusWaiting {
		for _, interrupt := range snapshot.Interrupts {
			t.showInterrupt(interrupt)
		}
		t.showUsage(target.Usage)
	}
	if target.Status == protocol.RunStatusFinished && !t.settled {
		t.finished(target)
		t.settled = true
	}
	return t.err
}

func (t *textRenderer) resume(block conversation.Block) {
	key := conversation.BlockKey(block.RunID, block.ID)
	if _, present := t.streaming[key]; present {
		return
	}
	if _, present := t.pending[key]; present {
		return
	}
	t.begin(block)
}

func (t *textRenderer) showInterrupt(interrupt conversation.Interrupt) {
	key := conversation.BlockKey(conversation.InterruptRunID(interrupt), conversation.InterruptItemID(interrupt))
	if _, duplicate := t.shown[key]; duplicate {
		return
	}
	t.shown[key] = struct{}{}
	t.interrupted(interrupt)
}

func (t *textRenderer) completedText(block conversation.Block) string {
	if block.Text != "" {
		return block.Text
	}
	if pending := t.pending[conversation.BlockKey(block.RunID, block.ID)]; pending != nil {
		return pending.body.String()
	}
	return ""
}

func (t *textRenderer) renderCompletedBlock(b conversation.Block, text string) {
	switch b.Kind {
	case conversation.BlockUser:
		t.userBlock(b, text)
	case conversation.BlockAssistant:
		t.proseBlock(b, text)
	case conversation.BlockReasoning:
		t.blank()
		t.block("· ", text)
	case conversation.BlockTool:
		t.tool(b)
	case conversation.BlockQuestion:
		if b.Question != nil {
			t.shown[conversation.BlockKey(b.RunID, b.ID)] = struct{}{}
			t.interrupted(*b.Question)
		}
	case conversation.BlockNotice:
		t.blank()
		t.block("! ", text)
	case conversation.BlockError:
		t.blank()
		t.block("× ", text)
	}
}

func (t *textRenderer) userBlock(block conversation.Block, text string) {
	t.blank()
	if text != "" {
		t.block("› ", text)
	}
	for _, attachment := range block.Attachments {
		t.line("  @ " + attachment.Name + " (" + attachment.MimeType + ", " + strconv.FormatInt(attachment.Size, 10) + " bytes)")
	}
}

func (t *textRenderer) proseBlock(block conversation.Block, text string) {
	t.blank()
	if t.scope.isChild(block.RunID) {
		t.line("subagent · " + block.RunID)
	}
	t.write(text)
	t.endLine()
	t.showImages(block.Images)
}

func (t *textRenderer) showImages(images []conversation.InlineImage) {
	for _, image := range images {
		t.line("  @ " + image.Name + " (" + image.MIMEType + ", " + strconv.Itoa(len(image.Data)) + " bytes)")
	}
}

func (t *textRenderer) tool(b conversation.Block) {
	call := b.Tool
	if call == nil {
		return
	}
	if call.Status == conversation.ToolRunning {
		return
	}
	t.blank()
	t.toolHeader(call)
	t.toolBody(call)
	t.toolVerdict(call)
}

func (t *textRenderer) toolHeader(call *conversation.ToolCall) {
	head := "● " + textToolName(call)
	if call.Safety != "" {
		head += format.Separator + string(call.Safety)
	}
	primary := textToolPrimary(call)
	if primary != "" {
		head += format.Separator + primary
	}
	if call.Summary != "" && call.Summary != primary {
		head += format.Separator + call.Summary
	}
	t.line(head)
}

func (t *textRenderer) toolBody(call *conversation.ToolCall) {
	if call.Output != "" {
		lines := strings.Split(strings.TrimRight(call.Output, "\n"), "\n")
		shown := min(len(lines), maxToolOutputLines)
		for _, l := range lines[:shown] {
			t.line("  │ " + l)
		}
		if rest := len(lines) - shown; rest > 0 {
			t.line("  │ … " + strconv.Itoa(rest) + " more lines")
		}
	}
}

func (t *textRenderer) toolVerdict(call *conversation.ToolCall) {
	mark := "✓"
	switch call.Status {
	case conversation.ToolError:
		mark = "✗"
	case conversation.ToolCanceled:
		mark = "−"
	case conversation.ToolRunning, conversation.ToolOK:
	}
	status := "  " + mark
	if call.Status == conversation.ToolCanceled {
		status += " canceled"
	}
	if call.ExitCode != nil && *call.ExitCode != 0 {
		status += " exit " + strconv.Itoa(*call.ExitCode)
	}
	if call.Duration > 0 {
		status += " " + format.Duration(call.Duration)
	}
	t.line(status)
}

func textToolName(call *conversation.ToolCall) string {
	if call.Kind == "" {
		if call.Name != "" {
			return call.Name
		}
		return "tool"
	}
	return string(call.Kind)
}

func textToolPrimary(call *conversation.ToolCall) string {
	var value string
	switch call.Kind {
	case builtin.CategoryShell:
		value = call.Command
	case builtin.CategoryEdit, builtin.CategoryRead:
		value = call.Path
	case builtin.CategorySearch:
		value = call.Query
	case "", builtin.CategoryTask:
	default:
	}
	if value != "" {
		return value
	}
	return call.Summary
}

func (t *textRenderer) plan(items []protocol.PlanStep) {
	if len(items) == 0 {
		return
	}
	t.blank()
	t.line("plan")
	for _, it := range items {
		mark := "☐"
		switch it.Status {
		case protocol.PlanStatusInProgress:
			mark = "▸"
		case protocol.PlanStatusCompleted:
			mark = "☑"
		case protocol.PlanStatusPending:
		default:
		}
		t.line("  " + mark + " " + it.Description)
	}
}

func (t *textRenderer) interrupted(interrupt conversation.Interrupt) {
	t.blank()
	switch item := interrupt.(type) {
	case conversation.Approval:
		t.line("? " + item.Title)
		if item.Detail != "" {
			t.block("  ", item.Detail)
		}
	case conversation.Question:
		t.line("? " + item.Title)
		for index, field := range item.Fields {
			t.line("  - " + field.Prompt)
			if item.Answered() {
				t.line("    answer: " + strings.Join(item.Answers[index], ", "))
			}
		}
	}
}

func (t *textRenderer) segmentFinished(runID string, event conversation.SegmentFinished) {
	root := t.scope.isRoot(runID)
	switch {
	case event.Run.Status == protocol.RunStatusFinished:
		if root {
			t.finished(event.Run)
			t.settled = true
		}
	case event.Suspended():
		if root {
			t.showUsage(event.Run.Usage)
		}
	default:
		for _, interrupt := range event.Interrupts {
			t.showInterrupt(interrupt)
		}
		t.showUsage(event.Run.Usage)
	}
}

func (t *textRenderer) finished(run conversation.Run) {
	t.blank()
	if run.Outcome.Status != protocol.OutcomeCompleted {
		msg := string(run.Outcome.Status)
		if detail := run.Outcome.Explanation(); detail != "" {
			msg += ": " + detail
		}
		t.line(msg)
	}
	t.showUsage(run.Usage)
}

func (t *textRenderer) showUsage(u conversation.Usage) {
	if summary := format.Usage(u); summary != "" {
		t.line(summary)
	}
}

// The indent counts runes, not bytes: the markers are multi-byte and single-width.
func (t *textRenderer) block(prefix, text string) {
	indent := strings.Repeat(" ", utf8.RuneCountInString(prefix))
	first := true
	for l := range strings.SplitSeq(strings.TrimRight(text, "\n"), "\n") {
		if first {
			first = false
			t.line(prefix + l)
			continue
		}
		t.line(indent + l)
	}
}

func (t *textRenderer) line(s string) {
	t.write(s)
	t.endLine()
}

func (t *textRenderer) blank() {
	t.endLine()
	t.write("\n")
	t.column = false
}

func (t *textRenderer) endLine() {
	if !t.column {
		return
	}
	t.write("\n")
	t.column = false
}

func (t *textRenderer) write(s string) {
	if t.err != nil || s == "" {
		return
	}
	if _, err := io.WriteString(t.w, s); err != nil {
		t.err = err
		return
	}
	t.column = !strings.HasSuffix(s, "\n")
}
