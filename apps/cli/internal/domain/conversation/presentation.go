package conversation

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/failure"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

var (
	ErrSessionNotFound      = errors.New("session not found")
	ErrRunNotFound          = errors.New("run not found")
	ErrInterruptNotOpen     = errors.New("interrupt not open")
	ErrStaleSegment         = errors.New("stale segment")
	ErrRunWaiting           = errors.New("run is waiting")
	ErrRunFinished          = errors.New("run is finished")
	ErrReplayCursorInvalid  = errors.New("event replay cursor is invalid")
	ErrReplayUnavailable    = errors.New("event replay unavailable")
	ErrSessionHasActiveRun  = errors.New("session has an active run")
	ErrSessionBusy          = errors.New("session is busy")
	ErrPluginChanged        = errors.New("plugin changed while the run was being prepared")
	ErrRevisionConflict     = errors.New("revision conflict")
	ErrEventConflict        = errors.New("event identity conflict")
	ErrCommandInProgress    = errors.New("command is still committing")
	ErrCommandConflict      = errors.New("command identity conflict")
	ErrCommandStoreMismatch = errors.New("command belongs to another runtime idempotency store")
	ErrDisconnected         = errors.New("runtime disconnected")
	ErrIncompatibleRuntime  = errors.New("runtime protocol is incompatible")
)

type BlockKind string

const (
	BlockUser      BlockKind = "user"
	BlockAssistant BlockKind = "assistant"
	BlockReasoning BlockKind = "reasoning"
	BlockQuestion  BlockKind = "question"
	BlockTool      BlockKind = "tool"
	BlockNotice    BlockKind = "notice"
	BlockError     BlockKind = "error"
)

type BlockStatus string

const (
	BlockStatusRunning    BlockStatus = BlockStatus(protocol.ItemStatusRunning)
	BlockStatusCompleted  BlockStatus = BlockStatus(protocol.ItemStatusCompleted)
	BlockStatusIncomplete BlockStatus = BlockStatus(protocol.ItemStatusIncomplete)
)

type Block struct {
	ID              string
	RunID           string
	Status          BlockStatus
	Kind            BlockKind
	CreatedAt       time.Time
	Redacted        bool
	DroppedMessages int
	Attachments     []authoring.Attachment
	Images          []InlineImage
	Text            string
	Question        *Question
	Tool            *ToolCall
}

const blockKeyLengthSeparator byte = ':'

func BlockKey(runID, blockID string) string {
	var encoded strings.Builder
	encoded.Grow(len(runID) + len(blockID) + 2*4)
	writeBlockKeyField(&encoded, runID)
	writeBlockKeyField(&encoded, blockID)
	return encoded.String()
}

func writeBlockKeyField(encoded *strings.Builder, value string) {
	encoded.WriteString(strconv.Itoa(len(value)))
	encoded.WriteByte(blockKeyLengthSeparator)
	encoded.WriteString(value)
}

func (b Block) Clone() Block {
	b.Attachments = slices.Clone(b.Attachments)
	b.Images = cloneInlineImages(b.Images)
	if b.Question != nil {
		question := cloneQuestion(*b.Question)
		b.Question = &question
	}
	if b.Tool != nil {
		tool := b.Tool.Clone()
		b.Tool = &tool
	}
	return b
}

func (b Block) Equal(other Block) bool {
	if b.ID != other.ID || b.RunID != other.RunID || b.Status != other.Status || b.Kind != other.Kind ||
		!b.CreatedAt.Equal(other.CreatedAt) || b.Redacted != other.Redacted || b.DroppedMessages != other.DroppedMessages ||
		b.Text != other.Text || !slices.Equal(b.Attachments, other.Attachments) || !equalInlineImages(b.Images, other.Images) {
		return false
	}
	return equalOptionalValue(b.Question, other.Question) && equalOptionalValue(b.Tool, other.Tool)
}

type InlineImage struct {
	ID       string
	Name     string
	MIMEType string
	Data     []byte
}

func (i InlineImage) Clone() InlineImage {
	i.Data = bytes.Clone(i.Data)
	return i
}

func (i InlineImage) Equal(other InlineImage) bool {
	return i.ID == other.ID && i.Name == other.Name && i.MIMEType == other.MIMEType &&
		bytes.Equal(i.Data, other.Data)
}

func cloneInlineImages(images []InlineImage) []InlineImage {
	if images == nil {
		return nil
	}
	cloned := make([]InlineImage, len(images))
	for index, image := range images {
		cloned[index] = image.Clone()
	}
	return cloned
}

func equalInlineImages(left, right []InlineImage) bool {
	return slices.EqualFunc(left, right, func(left, right InlineImage) bool { return left.Equal(right) })
}

type ToolStatus string

const (
	ToolRunning  ToolStatus = "running"
	ToolOK       ToolStatus = "ok"
	ToolError    ToolStatus = "error"
	ToolCanceled ToolStatus = "canceled"
)

func (s ToolStatus) valid() bool {
	return s == ToolRunning || s == ToolOK || s.Failed()
}

func (s ToolStatus) Failed() bool { return s == ToolError || s == ToolCanceled }

type ToolCall struct {
	Kind          builtin.ToolCategory       `json:"kind"`
	Name          string                     `json:"name"`
	Summary       string                     `json:"summary"`
	Status        ToolStatus                 `json:"status"`
	Safety        protocol.SafetyClass       `json:"safety"`
	StartedAt     time.Time                  `json:"startedAt"`
	FinishedAt    time.Time                  `json:"finishedAt"`
	Command       string                     `json:"command"`
	Path          string                     `json:"path"`
	Query         string                     `json:"query"`
	Output        string                     `json:"output"`
	ArgumentsJSON []byte                     `json:"argumentsJson"`
	ArgumentsText string                     `json:"argumentsText"`
	ResultJSON    []byte                     `json:"resultJson"`
	Problem       *protocol.ProblemData      `json:"problem"`
	ExitCode      *int                       `json:"exitCode"`
	OutputPage    *builtin.CommandOutputPage `json:"outputPage"`
	Duration      time.Duration              `json:"duration"`
}

func (t ToolCall) Clone() ToolCall {
	t.ArgumentsJSON = bytes.Clone(t.ArgumentsJSON)
	t.ResultJSON = bytes.Clone(t.ResultJSON)
	t.Problem = failure.Clone(t.Problem)
	if t.ExitCode != nil {
		t.ExitCode = new(*t.ExitCode)
	}
	if t.OutputPage != nil {
		t.OutputPage = new(*t.OutputPage)
	}
	return t
}

func (t ToolCall) Equal(other ToolCall) bool {
	if !t.sameInvocation(other) || t.Status != other.Status || t.Safety != other.Safety ||
		!t.StartedAt.Equal(other.StartedAt) || !t.FinishedAt.Equal(other.FinishedAt) ||
		t.Output != other.Output || !bytes.Equal(t.ResultJSON, other.ResultJSON) ||
		!failure.Equal(t.Problem, other.Problem) ||
		t.Duration != other.Duration {
		return false
	}
	return equalOptional(t.ExitCode, other.ExitCode) && equalOptional(t.OutputPage, other.OutputPage)
}

func (t ToolCall) sameInvocation(other ToolCall) bool {
	return t.Kind == other.Kind && t.Name == other.Name && t.Summary == other.Summary &&
		t.Command == other.Command && t.Path == other.Path && t.Query == other.Query &&
		t.ArgumentsText == other.ArgumentsText && bytes.Equal(t.ArgumentsJSON, other.ArgumentsJSON)
}

func (t ToolCall) Validate() error {
	if err := errors.Join(t.validateVocabulary(), t.validateTiming(), t.validatePayloads()); err != nil {
		return fmt.Errorf("tool call: %w", err)
	}
	return nil
}

func (t ToolCall) validateVocabulary() error {
	var problems []error
	if t.Kind != "" && !slices.Contains(t.Kind.Values(), string(t.Kind)) {
		problems = append(problems, fmt.Errorf("kind %q is invalid", t.Kind))
	}
	if !t.Status.valid() {
		problems = append(problems, fmt.Errorf("status %q is invalid", t.Status))
	}
	switch t.Safety {
	case "", protocol.SafetyClassSafe, protocol.SafetyClassWrite, protocol.SafetyClassExec, protocol.SafetyClassNetwork:
	default:
		problems = append(problems, fmt.Errorf("safety class %q is invalid", t.Safety))
	}
	if t.Kind == "" && strings.TrimSpace(t.Name) == "" {
		problems = append(problems, errors.New("unknown tool has no provider name"))
	}
	if t.Problem != nil && !t.Status.Failed() {
		problems = append(problems, errors.New("successful or running tool carries a problem"))
	}
	return errors.Join(problems...)
}

func (t ToolCall) validateTiming() error {
	var problems []error
	if t.Duration < 0 {
		problems = append(problems, errors.New("tool duration is negative"))
	}
	if t.Status == ToolRunning && (t.ExitCode != nil || t.Duration != 0 || !t.FinishedAt.IsZero()) {
		problems = append(problems, errors.New("running tool has a finish record"))
	}
	if !t.FinishedAt.IsZero() && (t.StartedAt.IsZero() || t.FinishedAt.Before(t.StartedAt)) {
		problems = append(problems, errors.New("tool finish time has no earlier start time"))
	}
	return errors.Join(problems...)
}

func (t ToolCall) validatePayloads() error {
	var problems []error
	if len(t.ArgumentsJSON) > 0 {
		var arguments map[string]jsontext.Value
		if !jsontext.Value(t.ArgumentsJSON).IsValid() || json.Unmarshal(t.ArgumentsJSON, &arguments) != nil || arguments == nil {
			problems = append(problems, errors.New("arguments JSON is not an object"))
		}
	}
	if len(t.ResultJSON) > 0 && !jsontext.Value(t.ResultJSON).IsValid() {
		problems = append(problems, errors.New("result JSON is invalid"))
	}
	return errors.Join(problems...)
}

type Outcome struct {
	Status  protocol.RunOutcomeType
	Problem *protocol.ProblemData
	Detail  string
	Loss    *protocol.RunLoss
}

func (o Outcome) Clone() Outcome {
	o.Problem = failure.Clone(o.Problem)
	if o.Loss != nil {
		o.Loss = new(*o.Loss)
	}
	return o
}

func (o Outcome) Equal(other Outcome) bool {
	return o.Status == other.Status && o.Detail == other.Detail && failure.Equal(o.Problem, other.Problem) &&
		(o.Loss == nil) == (other.Loss == nil) && (o.Loss == nil || *o.Loss == *other.Loss)
}

func (o Outcome) Description() string {
	if o.Problem != nil {
		return failure.Message(o.Problem, "")
	}
	if o.Loss != nil {
		return lossExplanation(o.Loss.Reason)
	}
	return strings.TrimSpace(o.Detail)
}

func (o Outcome) Explanation() string {
	if o.Problem != nil {
		return failure.String(o.Problem)
	}
	if o.Loss != nil {
		return lossExplanation(o.Loss.Reason)
	}
	return strings.TrimSpace(o.Detail)
}

func lossExplanation(reason protocol.LossReason) string {
	switch reason {
	case protocol.LossRestart:
		return "the Runtime stopped while the run was active"
	case protocol.LossIsolatedWorkspace:
		return "its isolated workspace does not survive a restart"
	case protocol.LossWorkspaceUnavailable:
		return "its workspace is unavailable"
	case protocol.LossOtherBuild:
		return "its waiting state was written by another Runtime build"
	case protocol.LossWaitingStateUnavailable:
		return "its waiting state could not be restored"
	case protocol.LossConfigurationChanged:
		return "its configuration changed while it was waiting"
	case protocol.LossUnresolvedEffects:
		return "external operations have no provable result"
	case protocol.LossUnresolvedModelCall:
		return "a model call ended without a provable result"
	default:
		return string(reason)
	}
}

func ApprovalSubject(subject protocol.ApprovalSubject) string {
	if subject.Type == protocol.ApprovalSubjectAll {
		return "all"
	}
	return string(subject.Type) + ":" + subject.Value
}
