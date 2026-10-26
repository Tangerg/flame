package transcript

import (
	"bytes"
	"slices"
)

// Equal compares the complete semantic Item, including accepted answers and
// terminal execution evidence, independently of codec slice representation.
func (i Item) Equal(other Item) bool {
	if i.ID() != other.ID() || i.SessionID() != other.SessionID() || i.RunID() != other.RunID() ||
		!i.OccurredAt().Equal(other.OccurredAt()) || !i.finishedAt.Equal(other.finishedAt) ||
		i.status != other.status || i.kind != other.kind || i.messagePhase != other.messagePhase ||
		i.text != other.text || i.redacted != other.redacted || i.safetyClass != other.safetyClass ||
		i.approvalDecision != other.approvalDecision || i.editedArguments != other.editedArguments ||
		i.denialReason != other.denialReason || i.rememberScope != other.rememberScope ||
		i.summary != other.summary || i.droppedMessages != other.droppedMessages {
		return false
	}
	if (i.executionDuration == nil) != (other.executionDuration == nil) || (i.failure == nil) != (other.failure == nil) {
		return false
	}
	if i.executionDuration != nil && *i.executionDuration != *other.executionDuration {
		return false
	}
	if i.failure != nil && *i.failure != *other.failure {
		return false
	}
	if !slices.EqualFunc(i.content, other.content, func(left, right ContentBlock) bool {
		return left.Kind == right.Kind && left.Text == right.Text && left.MediaType == right.MediaType &&
			bytes.Equal(left.Bytes, right.Bytes)
	}) {
		return false
	}
	if (i.question == nil) != (other.question == nil) || (i.tool == nil) != (other.tool == nil) {
		return false
	}
	if i.question != nil && !i.question.Equal(*other.question) {
		return false
	}
	return i.tool == nil || i.tool.Equal(*other.tool)
}
