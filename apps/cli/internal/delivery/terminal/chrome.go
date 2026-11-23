package terminal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	tokensPerThousand        = 1_000
	tokensPerMillion         = 1_000_000
	fractionalThousandsLimit = 10 * tokensPerThousand
	narrowActivityWidth      = 60
	narrowActivityMaxRows    = 4
	activityGutter           = 2
	activityLabelGap         = 2
)

const (
	headerMinWidth   = 32
	activityMinWidth = 28
	activityMaxRows  = 6
)

type sessionHeader struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	session conversation.Session
	usage   conversation.Usage
	goal    *protocol.Goal
	changes int
}

func newSessionHeader(theme kit.Theme, glyphs kit.Glyphs, session conversation.Session) *sessionHeader {
	return &sessionHeader{theme: theme, glyphs: glyphs, session: session}
}

func (s *sessionHeader) SetSession(session conversation.Session) { s.session = session }

func (s *sessionHeader) SetUsage(usage conversation.Usage) { s.usage = usage.Clone() }

func (s *sessionHeader) SetGoal(current *protocol.Goal) {
	if current == nil {
		s.goal = nil
		return
	}
	goal := *current
	s.goal = &goal
}

func (s *sessionHeader) SetWorkspaceChanges(count int) {
	s.changes = max(count, 0)
}

func (s *sessionHeader) HeightForWidth(width int) int {
	if width < headerMinWidth {
		return 0
	}
	return 2
}

func (s *sessionHeader) Draw(view grid.View) {
	width, height := view.Size()
	if width < headerMinWidth || height <= 0 {
		return
	}
	right := headerRightLabel(s.usage, s.changes)
	rightWidth := text.Width(right)
	if rightWidth > 0 && rightWidth < width {
		view.Text(width-rightWidth, 0, right, s.theme.Subtle)
	} else {
		rightWidth = 0
	}

	available := width
	if rightWidth > 0 {
		available -= rightWidth + 2
	}
	if available > 0 {
		workspace := displayWorkspace(s.session.Workspace)
		title := displayTitle(s.session)
		separator := "  " + s.glyphs.Bullet + "  "
		x := text.Of(workspace, s.theme.Context).Truncate(available, s.glyphs.Ellipsis).Draw(view, 0, 0)
		remaining := available - x
		if remaining > text.Width(separator) {
			x += view.Text(x, 0, separator, s.theme.Muted)
			text.Of(title, s.theme.Muted).Truncate(remaining-text.Width(separator), s.glyphs.Ellipsis).Draw(view, x, 0)
		}
	}
	if height > 1 {
		s.drawGoal(view.Sub(grid.Area(0, 1, width, 1)))
	}
}

func (s *sessionHeader) drawGoal(view grid.View) {
	if s.goal == nil {
		return
	}
	width, _ := view.Size()
	state := string(s.goal.Status)
	prefix := "[Goal: " + state + "]"
	style := s.theme.Accent
	switch s.goal.Status {
	case protocol.GoalPaused:
		style = s.theme.Warning
	case protocol.GoalBlocked:
		style = s.theme.Danger
	case protocol.GoalActive, protocol.GoalCompleting:
	}
	right := goalUsageLabel(s.goal.Used)
	rightWidth := text.Width(right)
	available := width
	if rightWidth > 0 && rightWidth < width {
		view.Text(width-rightWidth, 0, right, s.theme.Subtle)
		available -= rightWidth + 2
	}
	if available <= 0 {
		return
	}
	x := text.Of(prefix, style).Truncate(available, s.glyphs.Ellipsis).Draw(view, 0, 0)
	remaining := available - x
	if remaining > 2 {
		text.Of(s.goal.Objective, s.theme.Muted).Truncate(remaining-2, s.glyphs.Ellipsis).Draw(view, x+2, 0)
	}
}

func goalUsageLabel(used protocol.GoalUsage) string {
	var parts []string
	if used.Runs > 0 {
		parts = append(parts, countedNoun(used.Runs, "run"))
	}
	if used.Steps > 0 {
		parts = append(parts, countedNoun(used.Steps, "step"))
	}
	if used.CostUSD != nil {
		if *used.CostUSD > 0 {
			parts = append(parts, format.USD(*used.CostUSD))
		}
	} else if used.Runs > 0 {
		parts = append(parts, costUnavailable)
	}
	return strings.Join(parts, "  ")
}

func headerRightLabel(usage conversation.Usage, changes int) string {
	parts := make([]string, 0, 2)
	if tokens := headerUsageLabel(usage); tokens != "" {
		parts = append(parts, tokens)
	}
	if changes > 0 {
		parts = append(parts, fmt.Sprintf("Δ%d", changes))
	}
	return strings.Join(parts, "  ")
}

func displayWorkspace(value workspace.Workspace) string {
	path := value.Path
	if path == "." || path == "" {
		return "workspace"
	}
	if !value.IsAvailable() {
		return path + "  ·  missing"
	}
	return path
}

func headerUsageLabel(usage conversation.Usage) string {
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		return ""
	}
	return "↑" + compactTokens(usage.InputTokens) + "  ↓" + compactTokens(usage.OutputTokens)
}

func compactTokens(tokens int64) string {
	magnitude := float64(tokens)
	if magnitude < 0 {
		magnitude = -magnitude
	}
	switch {
	case magnitude < tokensPerThousand:
		return strconv.FormatInt(tokens, 10)
	case magnitude < fractionalThousandsLimit:
		return strconv.FormatFloat(float64(tokens)/tokensPerThousand, 'f', 1, 64) + "k"
	case magnitude < tokensPerMillion:
		return strconv.FormatInt(tokens/tokensPerThousand, 10) + "k"
	default:
		return strconv.FormatFloat(float64(tokens)/tokensPerMillion, 'f', 1, 64) + "m"
	}
}

type activityView struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	items  []protocol.PlanStep
}

func newActivityView(theme kit.Theme, glyphs kit.Glyphs) *activityView {
	return &activityView{theme: theme, glyphs: glyphs}
}

func (a *activityView) Set(items []protocol.PlanStep) {
	a.items = append(a.items[:0], items...)
}

func (a *activityView) Reset() {
	clear(a.items)
	a.items = a.items[:0]
}

func (a *activityView) HeightForWidth(width int) int {
	if len(a.items) == 0 || width < activityMinWidth {
		return 0
	}
	maximum := activityMaxRows
	if width < narrowActivityWidth {
		maximum = narrowActivityMaxRows
	}
	return min(len(a.items)+1, maximum)
}

func (a *activityView) Draw(view grid.View) {
	width, height := view.Size()
	if len(a.items) == 0 || width < activityMinWidth || height <= 0 {
		return
	}
	done, active := activityProgress(a.items)
	header := a.glyphs.Expanded + " Plan"
	view.Text(0, 0, header, a.theme.Heading)
	progress := fmt.Sprintf("%d/%d", done, len(a.items))
	if active >= 0 {
		progress = fmt.Sprintf("step %d/%d", active+1, len(a.items))
	}
	if progressWidth := text.Width(progress); progressWidth+text.Width(header)+2 <= width {
		view.Text(width-progressWidth, 0, progress, a.theme.Subtle)
	}

	capacity := height - 1
	start, end := activityWindow(len(a.items), capacity, active)
	for row, index := 1, start; row < height && index < end; row, index = row+1, index+1 {
		item := a.items[index]
		mark, label, style := a.glyphs.Free, "pending", a.theme.Muted
		switch item.Status {
		case protocol.PlanStatusInProgress:
			mark, label, style = a.glyphs.Marker, "active", a.theme.Accent
		case protocol.PlanStatusCompleted:
			mark, label, style = a.glyphs.Taken, "done", a.theme.Success
		case protocol.PlanStatusPending:
		default:
		}
		view.Text(0, row, a.glyphs.Vertical.String(), a.theme.Divider)
		labelWidth := text.Width(label)
		contentWidth := max(width-labelWidth-activityGutter-activityLabelGap, 1)
		x := activityGutter + view.Text(activityGutter, row, mark+" ", style)
		text.Of(item.Description, style).Truncate(contentWidth, a.glyphs.Ellipsis).Draw(view, x, row)
		if labelWidth+activityGutter+activityLabelGap < width {
			view.Text(width-labelWidth, row, label, style)
		}
	}
}

func activityProgress(items []protocol.PlanStep) (done, active int) {
	active = -1
	for index, item := range items {
		switch item.Status {
		case protocol.PlanStatusCompleted:
			done++
		case protocol.PlanStatusInProgress:
			active = index
		case protocol.PlanStatusPending:
		}
	}
	return done, active
}

func activityWindow(total, capacity, active int) (start, end int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	capacity = min(capacity, total)
	if active < 0 {
		return 0, capacity
	}
	start = active - capacity/2
	start = min(max(start, 0), total-capacity)
	return start, start + capacity
}

type statusView struct {
	theme              kit.Theme
	glyphs             kit.Glyphs
	doing              string
	problem            string
	elapsed            string
	usage              conversation.Usage
	contextTokens      int64
	outcome            conversation.Outcome
	status             kit.Status
	phase              statusPhase
	runningDescendants int
}

type statusPhase uint8

const (
	statusIdle statusPhase = iota
	statusActive
	statusFailed
)

const (
	idleActivity     = "ready"
	canceledActivity = "canceled"
)

func newStatusView(theme kit.Theme, glyphs kit.Glyphs) *statusView {
	return &statusView{theme: theme, glyphs: glyphs, doing: idleActivity}
}

func (s *statusView) Reset() {
	*s = statusView{theme: s.theme, glyphs: s.glyphs, doing: idleActivity, problem: s.problem}
}

func (s *statusView) HeightForWidth(int) int { return 1 }

func (s *statusView) Draw(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	if s.problem != "" {
		kit.Label{Text: statusLineText(s.problem), Style: s.theme.Danger, Ellipsis: s.glyphs.Ellipsis}.Draw(view)
		return
	}
	if s.phase == statusActive {
		s.status.Theme, s.status.Glyphs = s.theme, s.glyphs
		s.status.Label, s.status.Elapsed = statusLineText(s.doing), s.elapsed
		right := joinStatusLabels(
			contextLabel(s.contextTokens),
			runningDescendantsLabel(s.glyphs, s.runningDescendants),
		)
		rightWidth := text.Width(right)
		if right == "" || rightWidth+2 >= width {
			s.status.Draw(view)
			return
		}
		s.status.Draw(view.Sub(grid.Area(0, 0, width-rightWidth-1, 1)))
		view.Text(width-rightWidth, 0, right, s.theme.Subtle)
		return
	}
	right := joinStatusLabels(contextLabel(s.contextTokens), format.Usage(s.usage))
	style := s.theme.Muted
	switch {
	case s.phase == statusFailed:
		style = s.theme.Danger
	case s.outcome.Status == protocol.OutcomeCompleted:
		style = s.theme.Success
	case s.outcome.Status == protocol.OutcomeCanceled || s.outcome.Status == protocol.OutcomeTimedOut:
		style = s.theme.Warning
	case s.outcome.Status == protocol.OutcomeFailed || s.outcome.Status == protocol.OutcomeLost:
		style = s.theme.Danger
	}
	left := statusLineText(s.doing)
	if right == "" || text.Width(right)+2 >= width {
		kit.Label{Text: left, Style: style, Ellipsis: s.glyphs.Ellipsis}.Draw(view)
		return
	}
	rightWidth := text.Width(right)
	kit.Label{Text: left, Style: style, Ellipsis: s.glyphs.Ellipsis}.Draw(view.Sub(grid.Area(0, 0, width-rightWidth-1, 1)))
	view.Text(width-rightWidth, 0, right, s.theme.Subtle)
}

func (s *statusView) setProblem(problem string) { s.problem = strings.TrimSpace(problem) }

func (s *statusView) setRunningDescendants(count int) {
	s.runningDescendants = max(count, 0)
}

func runningDescendantsLabel(glyphs kit.Glyphs, count int) string {
	if count <= 0 {
		return ""
	}
	return glyphs.Bullet + " " + countedNoun(count, "subagent") + " active"
}

func joinStatusLabels(labels ...string) string {
	var joined string
	for _, label := range labels {
		if label == "" {
			continue
		}
		if joined != "" {
			joined += "  "
		}
		joined += label
	}
	return joined
}

func statusLineText(value string) string {
	return text.Printable(strings.Join(strings.Fields(value), " "))
}

func contextLabel(contextTokens int64) string {
	if contextTokens <= 0 {
		return ""
	}
	return "ctx " + format.Thousands(contextTokens)
}

func modelLabel(options authoring.RunOptions) string {
	if options.Provider != "" && options.Model != "" {
		label := options.Provider + "/" + options.Model
		if options.ReasoningEffort != "" {
			label += " · reasoning " + options.ReasoningEffort
		}
		return label
	}
	return "runtime default"
}

// Commands retain the empty pair so Runtime reads the active Session's durable
// selection instead of a second client-owned default.
func displayRunOptions(options authoring.RunOptions, session conversation.Session) authoring.RunOptions {
	if options.Provider == "" && options.Model == "" {
		options.Provider, options.Model = session.Provider, session.Model
		options.ReasoningEffort = session.ReasoningEffort
	}
	return options
}

func (s *statusView) tick(elapsed time.Duration) {
	s.status.Tick()
	s.elapsed = fmt.Sprintf("%4.1fs", elapsed.Seconds())
}

func (s *statusView) settled(run conversation.Run) {
	s.observeRun(run)
	s.outcome, s.elapsed = run.Outcome.Clone(), ""
	s.phase = statusIdle
	s.runningDescendants = 0
	switch run.Outcome.Status {
	case protocol.OutcomeCompleted:
		s.doing = "complete"
	case protocol.OutcomeCanceled:
		s.doing = canceledActivity
	case protocol.OutcomeTimedOut:
		s.doing = "timed out"
	case protocol.OutcomeFailed:
		s.doing = "failed: " + run.Outcome.Explanation()
	case protocol.OutcomeLost:
		s.doing = "lost: " + run.Outcome.Explanation()
	default:
		s.doing = idleActivity
	}
}

func (s *statusView) beginRun(label string) {
	s.usage = conversation.Usage{}
	s.contextTokens = 0
	s.active(label)
}

func (s *statusView) observeRun(run conversation.Run) {
	s.usage = run.Usage.Clone()
	s.contextTokens = run.ContextTokens
}

func (s *statusView) active(label string) {
	s.doing = label
	s.outcome = conversation.Outcome{}
	s.elapsed = ""
	s.phase = statusActive
}

func (s *statusView) progress(progress conversation.RunProgress) {
	if progress.Usage != nil {
		s.usage = progress.Usage.Clone()
	}
	if progress.ContextTokens != nil {
		s.contextTokens = *progress.ContextTokens
	}
	parts := []string{activityLabel(progress.Activity)}
	if progress.Step != nil {
		parts = append(parts, "step "+strconv.Itoa(*progress.Step))
	}
	s.active(strings.Join(parts, format.Separator))
}

func activityLabel(activity *protocol.Activity) string {
	if activity == nil {
		return statusWorking
	}
	switch activity.Type {
	case protocol.ActivityCallingModel:
		return "calling model"
	case protocol.ActivityModelCallFailed:
		return "model call failed"
	case protocol.ActivityApplyingUserInput:
		return "applying your input"
	case protocol.ActivityToolCall:
		return toolLabel(conversation.ToolCall{Name: activity.Tool})
	default:
		return statusWorking
	}
}

func (s *statusView) note(label string) {
	s.doing = label
	s.outcome = conversation.Outcome{}
	s.phase = statusIdle
}

func (s *statusView) fail(detail string, busy bool) {
	s.doing = "client failed: " + detail
	s.outcome = conversation.Outcome{}
	s.elapsed = ""
	s.phase = statusFailed
	if busy {
		s.phase = statusActive
	}
}
