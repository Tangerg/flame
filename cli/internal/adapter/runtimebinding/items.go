package runtimebinding

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func projectItem(value protocol.Item) (agent.Block, error) {
	projection := itemProjection{
		source: value,
		block: agent.Block{
			ID: value.ID, RunID: value.RunID, Status: agent.BlockStatus(value.Status), CreatedAt: value.CreatedAt,
		},
	}
	if err := projection.project(); err != nil {
		return agent.Block{}, err
	}
	if err := validateProjectedBlock(projection.block); err != nil {
		return agent.Block{}, fmt.Errorf("runtime item %s: %w", value.ID, err)
	}
	return projection.block, nil
}

type itemProjection struct {
	source protocol.Item
	block  agent.Block
}

func (i *itemProjection) project() error {
	switch i.source.Type {
	case protocol.ItemTypeUserMessage:
		i.block.Kind = agent.BlockUser
		return i.projectMessage(true)
	case protocol.ItemTypeAgentMessage:
		i.block.Kind = agent.BlockAssistant
		return i.projectMessage(false)
	case protocol.ItemTypeReasoning:
		i.block.Kind = agent.BlockReasoning
		i.block.Text = i.source.Text
		i.block.Redacted = i.source.Redacted
		if i.block.Redacted && strings.TrimSpace(i.block.Text) == "" {
			i.block.Text = "Reasoning redacted by provider."
		}
	case protocol.ItemTypeQuestion:
		i.block.Kind = agent.BlockQuestion
		question, err := projectQuestion(i.source.RunID, i.source.ID, i.source.Question)
		if err != nil {
			return err
		}
		i.block.Question = &question
	case protocol.ItemTypeToolCall:
		i.block.Kind = agent.BlockTool
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
	i.block.Kind = agent.BlockNotice
	i.block.Text = i.source.Summary
	i.block.DroppedMessages = i.source.DroppedMessages
	return nil
}

func validateProjectedBlock(block agent.Block) error {
	event := agent.Event(agent.BlockCompleted{Block: block})
	if block.Status == agent.BlockStatusRunning {
		event = agent.BlockStarted{Block: block}
	}
	return agent.ValidateEvent(event)
}

func projectQuestion(runID, itemID string, value *protocol.Question) (agent.Question, error) {
	if value == nil {
		return agent.Question{}, fmt.Errorf("question item %s has no payload", itemID)
	}
	question := agent.Question{
		RunID: runID, ItemID: itemID, Fields: make([]agent.QuestionField, 0, len(value.Fields)),
		Answers: make([][]string, len(value.Answers)),
	}
	if value.Answers == nil {
		question.Answers = nil
	} else {
		for index, answers := range value.Answers {
			question.Answers[index] = slices.Clone(answers)
		}
	}
	for _, field := range value.Fields {
		projected := agent.QuestionField{
			Prompt: field.Prompt, Header: field.Header, AllowCustom: field.AllowCustom,
			Options: make([]agent.QuestionOption, 0, len(field.Options)),
		}
		switch field.Type {
		case protocol.QuestionFieldText:
			projected.Kind = agent.QuestionText
		case protocol.QuestionFieldChoice:
			if field.Multiple {
				projected.Kind = agent.QuestionMulti
			} else {
				projected.Kind = agent.QuestionSingle
			}
		default:
			return agent.Question{}, fmt.Errorf("question item %s has unsupported field type %q", itemID, field.Type)
		}
		for _, option := range field.Options {
			projected.Options = append(projected.Options, agent.QuestionOption{
				Label: option.Label, Description: option.Description, Preview: option.Preview,
			})
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
		return agent.Question{}, err
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

func projectTool(projection toolProjection) (agent.ToolCall, error) {
	value := projection.invocation
	if value == nil {
		return agent.ToolCall{}, errors.New("tool payload is absent")
	}
	argumentsJSON, err := json.Marshal(value.Arguments)
	if err != nil {
		return agent.ToolCall{}, fmt.Errorf("encode tool arguments: %w", err)
	}
	arguments := decodeToolArgumentsMaterial(argumentsJSON)
	tool := agent.ToolCall{
		Kind: kindForTool(value.Name), Name: value.Name, Summary: arguments.summary(value.Name),
		Safety:    agent.ToolSafetyClass(projection.safety),
		StartedAt: projection.startedAt, FinishedAt: projection.finishedAt,
		Command: arguments.command(),
		Path:    arguments.path(),
		Query:   arguments.query(),
		URL:     arguments.url(),
	}
	tool.ArgumentsJSON = argumentsJSON
	if value.Result != nil {
		resultJSON, err := json.Marshal(value.Result)
		if err != nil {
			return agent.ToolCall{}, fmt.Errorf("encode tool result: %w", err)
		}
		tool.ResultJSON = resultJSON
		projectToolResult(&tool, resultJSON)
	}
	if projection.problem != nil {
		tool.Problem = projectRuntimeProblem(projection.problem)
	}
	if projection.durationMillis != nil {
		tool.Duration = time.Duration(*projection.durationMillis) * time.Millisecond
	}
	switch projection.status {
	case protocol.ItemStatusRunning:
		tool.Status = agent.ToolRunning
	case protocol.ItemStatusCompleted:
		tool.Status = agent.ToolOK
	case protocol.ItemStatusIncomplete:
		tool.Status = agent.ToolError
		if projection.problem != nil && (projection.problem.Type == protocol.ProblemDeniedByUser ||
			projection.problem.Type == protocol.ProblemChildRunCanceled || projection.problem.Type == protocol.ProblemToolCanceled) {
			tool.Status = agent.ToolCanceled
		}
	default:
		return agent.ToolCall{}, fmt.Errorf("tool status %q is unsupported", projection.status)
	}
	if projection.problem != nil && strings.TrimSpace(projection.problem.Detail) != "" {
		tool.Output = projection.problem.Detail
	}
	if err := tool.Validate(); err != nil {
		return agent.ToolCall{}, err
	}
	return tool, nil
}

const toolSummaryRuneLimit = 120

// builtInToolName is the CLI adapter's anti-corruption vocabulary for Runtime
// tool presentation. It is not a live catalog: unknown and MCP-provided names
// intentionally remain generic.
type builtInToolName string

const (
	builtInApplyPatch          builtInToolName = "apply_patch"
	builtInAskUser             builtInToolName = "ask_user"
	builtInCreateGoal          builtInToolName = "create_goal"
	builtInCreateSchedule      builtInToolName = "create_schedule"
	builtInDeleteSchedule      builtInToolName = "delete_schedule"
	builtInDelegateTask        builtInToolName = "delegate_task"
	builtInEnterPlanMode       builtInToolName = "enter_plan_mode"
	builtInExitPlanMode        builtInToolName = "exit_plan_mode"
	builtInGetGoal             builtInToolName = "get_goal"
	builtInGlob                builtInToolName = "glob"
	builtInGrep                builtInToolName = "grep"
	builtInHTTPRequest         builtInToolName = "http_request"
	builtInListSchedules       builtInToolName = "list_schedules"
	builtInListSkills          builtInToolName = "list_skills"
	builtInLoadSkill           builtInToolName = "load_skill"
	builtInLSP                 builtInToolName = "lsp"
	builtInProposeSkill        builtInToolName = "propose_skill"
	builtInRead                builtInToolName = "read"
	builtInReadShellOutput     builtInToolName = "read_shell_output"
	builtInReadSkillResource   builtInToolName = "read_skill_resource"
	builtInReadToolResult      builtInToolName = "read_tool_result"
	builtInReportGoalOutcome   builtInToolName = "report_goal_outcome"
	builtInSearchConversations builtInToolName = "search_conversations"
	builtInSearchMemory        builtInToolName = "search_memory"
	builtInSearchTools         builtInToolName = "search_tools"
	builtInSetPlan             builtInToolName = "set_plan"
	builtInShell               builtInToolName = "shell"
	builtInStopShell           builtInToolName = "stop_shell"
	builtInWebFetch            builtInToolName = "web_fetch"
	builtInWebSearch           builtInToolName = "web_search"
)

func kindForTool(name string) agent.ToolKind {
	switch builtInToolName(name) {
	case builtInShell, builtInReadShellOutput, builtInStopShell:
		return agent.ToolShell
	case builtInApplyPatch:
		return agent.ToolEdit
	case builtInRead, builtInReadSkillResource, builtInReadToolResult:
		return agent.ToolRead
	case builtInGlob, builtInGrep, builtInSearchMemory, builtInSearchConversations, builtInSearchTools, builtInLSP:
		return agent.ToolSearch
	case builtInWebSearch, builtInWebFetch, builtInHTTPRequest:
		return agent.ToolWeb
	case builtInAskUser, builtInDelegateTask, builtInCreateGoal, builtInGetGoal, builtInReportGoalOutcome,
		builtInCreateSchedule, builtInListSchedules, builtInDeleteSchedule, builtInLoadSkill, builtInListSkills,
		builtInProposeSkill, builtInEnterPlanMode, builtInExitPlanMode, builtInSetPlan:
		return agent.ToolTask
	default:
		return agent.ToolUnknown
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

func projectToolResult(tool *agent.ToolCall, encoded []byte) {
	material := decodeToolResultMaterial(encoded)
	tool.Output = material.output()
	if exitCode, ok := material.exitCode(); ok {
		tool.ExitCode = &exitCode
	}
	paths := material.changedPaths()
	if tool.Path == "" && len(paths) != 0 {
		tool.Path = paths[0]
	}
	if tool.Output == "" && len(paths) != 0 {
		tool.Output = strings.Join(paths, "\n")
	}
	if tool.Output == "" {
		tool.Output = formattedJSON(encoded)
	}
}

func integerValue(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), int64(int(number)) == number
	case float64:
		converted := int(number)
		return converted, float64(converted) == number
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil && int64(int(parsed)) == parsed
	default:
		return 0, false
	}
}
