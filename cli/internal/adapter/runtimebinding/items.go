package runtimebinding

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/failure"
	"github.com/Tangerg/flame/runtime/protocol"
)

func projectItem(value protocol.Item) (conversation.Block, error) {
	projection := itemProjection{
		source: value,
		block: conversation.Block{
			ID: value.ID, RunID: value.RunID, Status: conversation.BlockStatus(value.Status), CreatedAt: value.CreatedAt,
		},
	}
	if err := projection.project(); err != nil {
		return conversation.Block{}, err
	}
	if err := validateProjectedBlock(projection.block); err != nil {
		return conversation.Block{}, fmt.Errorf("runtime item %s: %w", value.ID, err)
	}
	return projection.block, nil
}

type itemProjection struct {
	source protocol.Item
	block  conversation.Block
}

func (i *itemProjection) project() error {
	switch i.source.Type {
	case protocol.ItemTypeUserMessage:
		i.block.Kind = conversation.BlockUser
		return i.projectMessage(true)
	case protocol.ItemTypeAgentMessage:
		i.block.Kind = conversation.BlockAssistant
		return i.projectMessage(false)
	case protocol.ItemTypeReasoning:
		i.block.Kind = conversation.BlockReasoning
		i.block.Text = i.source.Text
		i.block.Redacted = i.source.Redacted
		if i.block.Redacted && strings.TrimSpace(i.block.Text) == "" {
			i.block.Text = "Reasoning redacted by provider."
		}
	case protocol.ItemTypeQuestion:
		i.block.Kind = conversation.BlockQuestion
		question, err := projectQuestion(i.source.RunID, i.source.ID, i.source.Question)
		if err != nil {
			return err
		}
		i.block.Question = &question
	case protocol.ItemTypeToolCall:
		i.block.Kind = conversation.BlockTool
		tool, err := projectTool(toolProjection{
			invocation: i.source.Tool,
			status:     i.source.Status, safety: i.source.SafetyClass,
			startedAt: i.source.StartedAt, finishedAt: i.source.FinishedAt,
			durationMillis: i.source.DurationMillis, problem: i.source.Error,
		})
		if err != nil {
			return fmt.Errorf("item %s: %w", i.source.ID, err)
		}
		i.block.Tool = &tool
	case protocol.ItemTypeCompaction:
		return i.projectCompaction()
	default:
		return fmt.Errorf("item %s has unsupported type %q", i.source.ID, i.source.Type)
	}
	return nil
}

func (i *itemProjection) projectMessage(allowAttachments bool) error {
	if allowAttachments {
		text, attachments, err := projectContent(i.source.ID, i.source.Content)
		if err != nil {
			return err
		}
		i.block.Text = text
		i.block.Attachments = attachments
		return nil
	}
	text, images, err := projectAssistantContent(i.source.ID, i.source.Content)
	if err != nil {
		return err
	}
	i.block.Text = text
	i.block.Images = images
	return nil
}

func (i *itemProjection) projectCompaction() error {
	if strings.TrimSpace(i.source.Summary) == "" {
		return fmt.Errorf("item %s has an empty compaction summary", i.source.ID)
	}
	if i.source.Summary != strings.TrimSpace(i.source.Summary) {
		return fmt.Errorf("item %s has a non-canonical compaction summary", i.source.ID)
	}
	i.block.Kind = conversation.BlockNotice
	i.block.Text = i.source.Summary
	i.block.DroppedMessages = i.source.DroppedMessages
	return nil
}

func validateProjectedBlock(block conversation.Block) error {
	event := conversation.Event(conversation.BlockCompleted{Block: block})
	if block.Status == conversation.BlockStatusRunning {
		event = conversation.BlockStarted{Block: block}
	}
	return conversation.ValidateEvent(event)
}

func projectQuestion(runID, itemID string, value *protocol.Question) (conversation.Question, error) {
	if value == nil {
		return conversation.Question{}, fmt.Errorf("question item %s has no payload", itemID)
	}
	question := conversation.Question{
		RunID: runID, ItemID: itemID, Fields: make([]conversation.QuestionField, 0, len(value.Fields)),
		Answers: conversation.CloneAnswers(value.Answers),
	}
	for _, field := range value.Fields {
		projected := conversation.QuestionField{
			Prompt: field.Prompt, Header: field.Header, AllowCustom: field.AllowCustom,
			Options: slices.Clone(field.Options),
		}
		switch field.Type {
		case protocol.QuestionFieldText:
			projected.Kind = conversation.QuestionText
		case protocol.QuestionFieldChoice:
			if field.Multiple {
				projected.Kind = conversation.QuestionMulti
			} else {
				projected.Kind = conversation.QuestionSingle
			}
		default:
			return conversation.Question{}, fmt.Errorf("question item %s has unsupported field type %q", itemID, field.Type)
		}
		question.Fields = append(question.Fields, projected)
	}
	for _, field := range question.Fields {
		if strings.TrimSpace(field.Header) != "" {
			question.Title = field.Header
			break
		}
	}
	if question.Title == "" {
		question.Title = "Question"
	}
	if err := question.Validate(); err != nil {
		return conversation.Question{}, err
	}
	return question, nil
}

type toolProjection struct {
	invocation     *protocol.ToolInvocation
	status         protocol.ItemStatus
	safety         protocol.SafetyClass
	startedAt      time.Time
	finishedAt     time.Time
	durationMillis *int64
	problem        *protocol.ProblemData
}

func projectTool(projection toolProjection) (conversation.ToolCall, error) {
	value := projection.invocation
	if value == nil {
		return conversation.ToolCall{}, errors.New("tool payload is absent")
	}
	argumentsJSON, err := encodeProjection(value.Arguments)
	if err != nil {
		return conversation.ToolCall{}, fmt.Errorf("encode tool arguments: %w", err)
	}
	tool := conversation.ToolCall{
		Kind: kindForTool(value.Name), Name: value.Name, Summary: toolSummary(value.Name, value.Arguments),
		Safety:    projection.safety,
		StartedAt: projection.startedAt, FinishedAt: projection.finishedAt,
		Command: toolText(value.Arguments, "command"),
		Path:    toolText(value.Arguments, "path", "file", "filename"),
		Query:   toolText(value.Arguments, "query", "pattern", "search"),
		URL:     toolText(value.Arguments, "url", "uri"),
	}
	tool.ArgumentsJSON = argumentsJSON
	tool.ArgumentsText = value.ArgumentsText
	if value.Result != nil {
		resultJSON, err := encodeProjection(value.Result)
		if err != nil {
			return conversation.ToolCall{}, fmt.Errorf("encode tool result: %w", err)
		}
		tool.ResultJSON = resultJSON
		projectToolResult(&tool, value.Result)
	}
	if projection.problem != nil {
		tool.Problem = failure.Clone(projection.problem)
	}
	if projection.durationMillis != nil {
		tool.Duration = time.Duration(*projection.durationMillis) * time.Millisecond
	}
	switch projection.status {
	case protocol.ItemStatusRunning:
		tool.Status = conversation.ToolRunning
	case protocol.ItemStatusCompleted:
		tool.Status = conversation.ToolOK
	case protocol.ItemStatusIncomplete:
		tool.Status = conversation.ToolError
		if projection.problem != nil && (projection.problem.Type == protocol.ProblemDeniedByUser ||
			projection.problem.Type == protocol.ProblemChildRunCanceled || projection.problem.Type == protocol.ProblemToolCanceled) {
			tool.Status = conversation.ToolCanceled
		}
	default:
		return conversation.ToolCall{}, fmt.Errorf("tool status %q is unsupported", projection.status)
	}
	if projection.problem != nil && strings.TrimSpace(projection.problem.Detail) != "" {
		tool.Output = projection.problem.Detail
	}
	if err := tool.Validate(); err != nil {
		return conversation.ToolCall{}, err
	}
	return tool, nil
}

const toolSummaryRuneLimit = 120

// builtInToolName is the CLI adapter's anti-corruption vocabulary for Runtime
// tool presentation. It is not a live catalog: unknown and MCP-provided names
// intentionally remain generic.
type builtInToolName string

const (
	builtInApplyPatch        builtInToolName = "apply_patch"
	builtInAskUser           builtInToolName = "ask_user"
	builtInCreateGoal        builtInToolName = "create_goal"
	builtInCreateSchedule    builtInToolName = "create_schedule"
	builtInDeleteSchedule    builtInToolName = "delete_schedule"
	builtInDelegateTask      builtInToolName = "delegate_task"
	builtInEnterPlanMode     builtInToolName = "enter_plan_mode"
	builtInExitPlanMode      builtInToolName = "exit_plan_mode"
	builtInGetGoal           builtInToolName = "get_goal"
	builtInGlob              builtInToolName = "glob"
	builtInGrep              builtInToolName = "grep"
	builtInHTTPRequest       builtInToolName = "http_request"
	builtInListSchedules     builtInToolName = "list_schedules"
	builtInListSkills        builtInToolName = "list_skills"
	builtInLoadSkill         builtInToolName = "load_skill"
	builtInLSP               builtInToolName = "lsp"
	builtInProposeSkill      builtInToolName = "propose_skill"
	builtInRead              builtInToolName = "read"
	builtInReadShellOutput   builtInToolName = "read_shell_output"
	builtInReadSkillResource builtInToolName = "read_skill_resource"
	builtInReadToolResult    builtInToolName = "read_tool_result"
	builtInReportGoalOutcome builtInToolName = "report_goal_outcome"
	builtInSearchMemory      builtInToolName = "search_memory"
	builtInSearchTools       builtInToolName = "search_tools"
	builtInSetPlan           builtInToolName = "set_plan"
	builtInShell             builtInToolName = "shell"
	builtInStopShell         builtInToolName = "stop_shell"
	builtInWebFetch          builtInToolName = "web_fetch"
	builtInWebSearch         builtInToolName = "web_search"
)

func kindForTool(name string) conversation.ToolKind {
	switch builtInToolName(name) {
	case builtInShell, builtInReadShellOutput, builtInStopShell:
		return conversation.ToolShell
	case builtInApplyPatch:
		return conversation.ToolEdit
	case builtInRead, builtInReadSkillResource, builtInReadToolResult:
		return conversation.ToolRead
	case builtInGlob, builtInGrep, builtInSearchMemory, builtInSearchTools, builtInLSP:
		return conversation.ToolSearch
	case builtInWebSearch, builtInWebFetch, builtInHTTPRequest:
		return conversation.ToolWeb
	case builtInAskUser, builtInDelegateTask, builtInCreateGoal, builtInGetGoal, builtInReportGoalOutcome,
		builtInCreateSchedule, builtInListSchedules, builtInDeleteSchedule, builtInLoadSkill, builtInListSkills,
		builtInProposeSkill, builtInEnterPlanMode, builtInExitPlanMode, builtInSetPlan:
		return conversation.ToolTask
	default:
		return conversation.ToolUnknown
	}
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}
