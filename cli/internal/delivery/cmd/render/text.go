// Package render writes a run's events out for a reader — a person at a
// terminal, or a program on the other end of a pipe.
//
// The renderers here are write-only projections. Text remembers only the live
// block identities needed to route deltas, NDJSON writes each event directly, and
// ResultJSON retains only final assistant prose. None keeps a full transcript;
// holding the whole conversation in memory is the TUI's job.
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

// maxToolOutputLines caps how much of a tool's output is shown. A test run that
// prints ten thousand lines should not bury the answer that follows it.
const maxToolOutputLines = 12

// Text renders events as plain text, with no color and no cursor movement, so
// the same bytes are correct on a terminal, in a pipe and in a log file.
//
// Assistant prose is written as it streams, because that is the content a reader
// is waiting for. Everything else is held until its block completes and then
// printed as a labeled unit — a marker prefix cannot survive being interleaved
// with a live token feed.
type Text struct {
	w     io.Writer
	err   error
	scope runScope

	// streaming contains assistant blocks whose ordered deltas are written
	// straight through until authoritative completion reconciles the result.
	streaming map[string]*plainTextStream
	// pending collects blocks that print only on completion. Retaining the kind
	// lets this projection enforce the same delta semantics as the aggregate.
	pending map[string]*pendingTextBlock
	seen    map[string]struct{}
	shown   map[string]struct{}
	settled bool
	// column tracks whether the cursor sits mid-line, so separators can be
	// inserted without doubling blank lines.
	column bool
}

type plainTextStream struct {
	text    conversation.StreamedText
	emitted strings.Builder
}

type pendingTextBlock struct {
	body strings.Builder
}

// NewText builds a plain-text renderer over w.
func NewText(w io.Writer) *Text {
	return &Text{
		w: w, streaming: make(map[string]*plainTextStream),
		pending: make(map[string]*pendingTextBlock), seen: make(map[string]struct{}),
		shown: make(map[string]struct{}),
	}
}

// Begin binds subsequent live and recovered output to the accepted run. Text
// does not print the identity, but retaining it prevents a cold read from
// accidentally selecting a newer run in the same session.
func (t *Text) Begin(run conversation.Run, _ prompt.RunOptions) error {
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

// Render writes one event. The first error is remembered and returned by every
// later call, so a caller may render a whole run and check once.
func (t *Text) Render(envelope conversation.RunEvent) error {
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

func (t *Text) renderEvent(envelope conversation.RunEvent) {
	switch event := envelope.Event.(type) {
	case conversation.SegmentStarted:
		// A run's identity is machinery, not content.
	case conversation.BlockStarted:
		t.begin(event.Block)
	case conversation.BlockDelta:
		t.delta(envelope.RunID, event)
	case conversation.ToolArgumentsDelta, conversation.RunProgress, conversation.CustomEvent:
		// These previews are available in NDJSON and the interactive terminal.
		// Plain text stays focused on human-readable transcript content.
	case conversation.BlockCompleted:
		t.finish(event.Block)
	case conversation.PlanChanged:
		t.plan(event.Plan.State.Steps)
	case conversation.RunInterrupted:
		for _, interrupt := range event.Interrupts {
			t.showInterrupt(interrupt)
		}
		t.showUsage(event.Usage)
	case conversation.RunSuspended:
		if t.scope.isRoot(envelope.RunID) {
			t.showUsage(event.Usage)
		}
	case conversation.RunFinished:
		if t.scope.isRoot(envelope.RunID) {
			t.finished(event)
			t.settled = true
		}
	default:
		t.err = fmt.Errorf("render text event: unsupported event %T", event)
	}
}

// Close ends the output on its own line.
func (t *Text) Close() error {
	t.endLine()
	return t.err
}

func (t *Text) begin(b conversation.Block) {
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

func (t *Text) delta(runID string, d conversation.BlockDelta) {
	key := conversation.BlockKey(runID, d.BlockID)
	if stream := t.streaming[key]; stream != nil {
		if err := stream.text.Apply(d); err != nil {
			t.err = fmt.Errorf("render text delta %s: %w", d.BlockID, err)
			return
		}
		t.write(d.Text)
		stream.emitted.WriteString(d.Text)
		return
	}
	if pending := t.pending[key]; pending != nil {
		pending.body.WriteString(d.Text)
	}
}

func (t *Text) finish(b conversation.Block) {
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

// Reconcile replaces missing streamed facts with an authoritative cold-read
// projection after replay is no longer possible. Already rendered blocks and
// interrupts are not printed twice.
func (t *Text) Reconcile(snapshot conversation.SessionSnapshot) error {
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
		t.finished(conversation.RunFinished{Outcome: target.Outcome, Usage: target.Usage})
		t.settled = true
	}
	return t.err
}

func (t *Text) resume(block conversation.Block) {
	key := conversation.BlockKey(block.RunID, block.ID)
	if _, present := t.streaming[key]; present {
		return
	}
	if _, present := t.pending[key]; present {
		return
	}
	t.begin(block)
}

func (t *Text) showInterrupt(interrupt conversation.Interrupt) {
	key := conversation.BlockKey(conversation.InterruptRunID(interrupt), conversation.InterruptItemID(interrupt))
	if _, duplicate := t.shown[key]; duplicate {
		return
	}
	t.shown[key] = struct{}{}
	t.interrupted(interrupt)
}

func (t *Text) completedText(block conversation.Block) string {
	if block.Text != "" {
		return block.Text
	}
	if pending := t.pending[conversation.BlockKey(block.RunID, block.ID)]; pending != nil {
		return pending.body.String()
	}
	return ""
}

func (t *Text) renderCompletedBlock(b conversation.Block, text string) {
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

func (t *Text) userBlock(block conversation.Block, text string) {
	t.blank()
	if text != "" {
		t.block("› ", text)
	}
	for _, attachment := range block.Attachments {
		t.line("  @ " + attachment.Name + " (" + attachment.MimeType + ", " + strconv.FormatInt(attachment.Size, 10) + " bytes)")
	}
}

func (t *Text) proseBlock(block conversation.Block, text string) {
	t.blank()
	if t.scope.isChild(block.RunID) {
		t.line("subagent · " + block.RunID)
	}
	t.write(text)
	t.endLine()
	t.showImages(block.Images)
}

func (t *Text) showImages(images []conversation.InlineImage) {
	for _, image := range images {
		t.line("  @ " + image.Name + " (" + image.MIMEType + ", " + strconv.Itoa(len(image.Data)) + " bytes)")
	}
}

func (t *Text) tool(b conversation.Block) {
	call := b.Tool
	if call == nil {
		return
	}
	// A running tool announces itself only once its result is in: printing a
	// header, then a body arriving later, reads as two events rather than one.
	if call.Status == conversation.ToolRunning {
		return
	}
	t.blank()
	t.toolHeader(call)
	t.toolBody(call)
	t.toolVerdict(call)
}

func (t *Text) toolHeader(call *conversation.ToolCall) {
	head := "● " + textToolName(call)
	if call.Safety != "" {
		head += " · " + string(call.Safety)
	}
	primary := textToolPrimary(call)
	if primary != "" {
		head += " · " + primary
	}
	if call.Summary != "" && call.Summary != primary {
		head += " · " + call.Summary
	}
	t.line(head)
}

func (t *Text) toolBody(call *conversation.ToolCall) {
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
	if call.Diff != "" {
		t.diff(call.Diff)
	}
}

func (t *Text) toolVerdict(call *conversation.ToolCall) {
	// The verdict goes last, under what it is a verdict on.
	mark := "✓"
	switch call.Status {
	case conversation.ToolError:
		mark = "✗"
	case conversation.ToolCanceled:
		mark = "−"
	}
	status := "  " + mark
	if call.Status == conversation.ToolCanceled {
		status += " canceled"
	}
	if call.ExitCode != nil && *call.ExitCode != 0 {
		status += " exit " + strconv.Itoa(*call.ExitCode)
	}
	if call.Duration > 0 {
		status += " " + formatDuration(call.Duration)
	}
	t.line(status)
}

func textToolName(call *conversation.ToolCall) string {
	switch call.Kind {
	case conversation.ToolShell:
		return "shell"
	case conversation.ToolEdit:
		return "edit"
	case conversation.ToolRead:
		return "read"
	case conversation.ToolSearch:
		return "search"
	case conversation.ToolWeb:
		return "web"
	case conversation.ToolTask:
		return "task"
	case conversation.ToolUnknown:
		if call.Name != "" {
			return call.Name
		}
		return "tool"
	default:
		return "tool"
	}
}

func textToolPrimary(call *conversation.ToolCall) string {
	var value string
	switch call.Kind {
	case conversation.ToolShell:
		value = call.Command
	case conversation.ToolEdit, conversation.ToolRead:
		value = call.Path
	case conversation.ToolSearch:
		value = call.Query
	case conversation.ToolWeb:
		value = call.URL
	case conversation.ToolUnknown, conversation.ToolTask:
		// These kinds have no more specific primary field.
	default:
	}
	if value != "" {
		return value
	}
	return call.Summary
}

func (t *Text) diff(d string) {
	for l := range strings.SplitSeq(strings.TrimRight(d, "\n"), "\n") {
		t.line("  " + l)
	}
}

func (t *Text) plan(items []protocol.PlanStep) {
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
			// The empty checkbox is already selected.
		default:
		}
		t.line("  " + mark + " " + it.Description)
	}
}

func (t *Text) interrupted(interrupt conversation.Interrupt) {
	t.blank()
	switch item := interrupt.(type) {
	case conversation.Approval:
		t.line("? " + item.Title)
		if item.Detail != "" {
			t.block("  ", item.Detail)
		}
		if item.Diff != "" {
			t.diff(item.Diff)
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

func (t *Text) finished(e conversation.RunFinished) {
	t.blank()
	if e.Outcome.Status != protocol.OutcomeCompleted {
		msg := string(e.Outcome.Status)
		if detail := e.Outcome.Explanation(); detail != "" {
			msg += ": " + detail
		}
		t.line(msg)
	}
	t.showUsage(e.Usage)
}

func (t *Text) showUsage(u conversation.Usage) {
	parts := []string{"↑ " + formatThousands(u.InputTokens), "↓ " + formatThousands(u.OutputTokens)}
	if u.CacheReadTokens > 0 {
		parts = append(parts, "cached "+formatThousands(u.CacheReadTokens))
	}
	if u.CostUSD != nil {
		parts = append(parts, "$"+strconv.FormatFloat(*u.CostUSD, 'f', 4, 64))
	}
	if u.Steps > 0 {
		parts = append(parts, "steps "+strconv.Itoa(u.Steps))
	}
	if u.Duration > 0 {
		parts = append(parts, formatDuration(u.Duration))
	}
	t.line(strings.Join(parts, "  "))
}

// block writes a body with prefix on the first line and matching indentation on
// the rest, so a multi-line body stays visually one thing.
//
// The indent counts runes, not bytes: the markers here are multi-byte and
// single-width, so a byte count would over-indent every continuation line by the
// marker's encoded length.
func (t *Text) block(prefix, text string) {
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

func (t *Text) line(s string) {
	t.write(s)
	t.endLine()
}

// blank opens a visual gap before a new unit, without stacking gaps.
func (t *Text) blank() {
	t.endLine()
	t.write("\n")
	t.column = false
}

// endLine closes the current line if one is open.
func (t *Text) endLine() {
	if !t.column {
		return
	}
	t.write("\n")
	t.column = false
}

func (t *Text) write(s string) {
	if t.err != nil || s == "" {
		return
	}
	if _, err := io.WriteString(t.w, s); err != nil {
		t.err = err
		return
	}
	t.column = !strings.HasSuffix(s, "\n")
}

// formatDuration prints a span the way a person reads one: sub-second in
// milliseconds, otherwise seconds with one decimal.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

// formatThousands groups an integer for reading.
func formatThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	first := 0
	if s[0] == '-' {
		first = 1
	}
	var b strings.Builder
	b.Grow(len(s) + (len(s)-first-1)/3)
	b.WriteString(s[:first])
	for i := first; i < len(s); i++ {
		if i > first && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
