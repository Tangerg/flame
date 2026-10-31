package runs

import (
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// Retention accounting bounds the variable heap held by the replay window. The
// event count separately bounds fixed headers, while these conservative entry
// charges cover slice and map backing storage even when their strings are empty.
// Values are intentionally estimates: they describe retained memory, not any
// durable or presentation encoding.
const (
	retainedEventHeaderBytes    = 128
	retainedRunHeaderBytes      = 256
	retainedItemHeaderBytes     = 192
	retainedSliceEntryBytes     = 64
	retainedMapEntryBytes       = 96
	retainedQuestionFieldBytes  = 96
	retainedQuestionOptionBytes = 64
	retainedInterruptBytes      = 128
	retainedEffectHeaderBytes   = 128
	retainedPlanEntryBytes      = 64
)

// retainedBytes is charged exactly once, when the event is published. Eviction
// and replay reuse the charge stored beside the event.
func retainedBytes(event Event) int {
	if event.Payload == nil {
		return retainedEventHeaderBytes + len(event.RunID) + len(event.SegmentID) + len(event.Cursor)
	}
	return retainedEventHeaderBytes + len(event.RunID) + len(event.SegmentID) + len(event.Cursor) +
		event.Payload.retainedBytes()
}

func retainedRunBytes(value run.Run) int {
	snapshot := value.Snapshot()
	bytes := retainedRunHeaderBytes + stringsBytes(
		snapshot.SessionID,
		snapshot.ID,
		snapshot.Lineage.SpawnedByItemID(),
		snapshot.Lineage.ParentRunID(),
		snapshot.Lineage.RootRunID(),
		snapshot.GoalIncarnationID,
		snapshot.ActiveSegmentID,
		snapshot.ModelSelection.Provider(),
		snapshot.ModelSelection.Model(),
		snapshot.ModelSelection.ReasoningEffort(),
	)
	switch ending := snapshot.Ending.(type) {
	case run.Canceled:
		bytes += len(ending.Detail)
	case run.TimedOut:
		bytes += len(ending.Detail)
	case run.Failed:
		bytes += len(ending.Failure.Detail) + len(ending.Failure.DocURL)
	case run.Lost:
		bytes += len(ending.Loss.Detail)
	}
	bytes += cap(snapshot.UnresolvedEffects) * retainedEffectHeaderBytes
	for _, effect := range snapshot.UnresolvedEffects {
		bytes += stringsBytes(effect.ProcessID(), effect.EffectID(), effect.Cause(), effect.Reason(), effect.Detail(), effect.Output())
	}
	usage, _ := snapshot.Metrics.Usage()
	bytes += retainedUsageBytes(&usage)
	bytes += cap(snapshot.Capabilities.InterruptKinds) * retainedSliceEntryBytes
	return bytes
}

func retainedItemBytes(item transcript.Item) int {
	bytes := retainedItemHeaderBytes + stringsBytes(item.SessionID(), item.ID(), item.RunID())
	switch payload := item.Body().(type) {
	case transcript.UserMessageBody:
		bytes += retainedContentBytes(payload.Content)
	case transcript.AgentMessageBody:
		bytes += retainedContentBytes(payload.Content)
	case transcript.ReasoningBody:
		bytes += len(payload.Text)
	case transcript.QuestionBody:
		bytes += retainedQuestionBytes(&payload.Question)
	case transcript.CompactionBody:
		bytes += len(payload.Summary)
	case transcript.ToolCallBody:
		bytes += len(payload.SafetyClass)
		if payload.Approval != nil {
			bytes += stringsBytes(string(payload.Approval.Decision), payload.Approval.EditedArguments,
				payload.Approval.Reason, string(payload.Approval.RememberScope))
		}
		bytes += retainedToolInvocationBytes(&payload.Invocation)
		bytes += retainedToolFailureBytes(payload.Failure)
	}
	return bytes
}

func retainedContentBytes(content []transcript.ContentBlock) int {
	bytes := cap(content) * retainedSliceEntryBytes
	for _, block := range content {
		bytes += len(block.Text) + len(block.MediaType) + cap(block.Bytes)
	}
	return bytes
}

func retainedItemStartBytes(item ItemStart) int {
	bytes := retainedItemHeaderBytes + stringsBytes(
		item.SessionID,
		item.RunID,
		item.ItemID,
		string(item.SafetyClass),
	)
	bytes += retainedToolInvocationBytes(item.ToolInvocation)
	return bytes
}

func retainedPlanSnapshotBytes(snapshot PlanSnapshot) int {
	bytes := retainedEventHeaderBytes + len(snapshot.SessionID) + cap(snapshot.Steps)*retainedPlanEntryBytes
	for _, item := range snapshot.Steps {
		bytes += len(item.Description)
	}
	return bytes
}

func retainedInterruptPayloadBytes(interrupt transcript.Interrupt) int {
	bytes := len(interrupt.ItemID) + len(interrupt.RunID)
	switch prompt := interrupt.Prompt.(type) {
	case transcript.Approval:
		bytes += retainedToolInvocationBytes(&prompt.Tool)
		bytes += len(prompt.Risk) + len(prompt.Reason)
	case transcript.Question:
		bytes += retainedQuestionBytes(&prompt)
	}
	return bytes
}

func retainedQuestionBytes(question *transcript.Question) int {
	if question == nil {
		return 0
	}
	bytes := cap(question.Fields) * retainedQuestionFieldBytes
	for _, field := range question.Fields {
		bytes += stringsBytes(field.Prompt, field.Header)
		bytes += cap(field.Options) * retainedQuestionOptionBytes
		for _, option := range field.Options {
			bytes += stringsBytes(option.Label, option.Description, option.Preview)
		}
	}
	bytes += cap(question.Answers) * retainedSliceEntryBytes
	for _, values := range question.Answers {
		bytes += cap(values)*retainedSliceEntryBytes + stringsBytes(values...)
	}
	return bytes
}

func retainedToolInvocationBytes(invocation *transcript.ToolInvocation) int {
	if invocation == nil {
		return 0
	}
	rejected, _ := invocation.Input.Rejected()
	bytes := len(invocation.Name) + len(invocation.Input.Arguments().Canonical()) + len(rejected)
	if invocation.Result != nil {
		bytes += len(invocation.Result.Canonical())
	}
	if invocation.Offload != nil {
		bytes += len(invocation.Offload.ID.String())
	}
	return bytes
}

func retainedToolFailureBytes(failure *tool.Failure) int {
	if failure == nil {
		return 0
	}
	return len(failure.Detail) + len(failure.DocURL)
}

func retainedUsageBytes(usage *accounting.Usage) int {
	if usage == nil {
		return 0
	}
	bytes := len(usage.ByModel) * retainedMapEntryBytes
	for model := range usage.ByModel {
		bytes += len(model)
	}
	return bytes
}

func stringsBytes(values ...string) int {
	var total int
	for _, value := range values {
		total += len(value)
	}
	return total
}
