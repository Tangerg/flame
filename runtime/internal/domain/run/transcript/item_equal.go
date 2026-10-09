package transcript

import (
	"bytes"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/optional"
)

// Equal compares the complete semantic Item, including accepted answers and
// terminal execution evidence, independently of codec slice representation.
func (i Item) Equal(other Item) bool {
	if i.ID() != other.ID() || i.SessionID() != other.SessionID() || i.RunID() != other.RunID() ||
		!i.OccurredAt().Equal(other.OccurredAt()) || !i.finishedAt.Equal(other.finishedAt) ||
		i.status != other.status || i.kind != other.kind || i.messagePhase != other.messagePhase ||
		i.text != other.text || i.redacted != other.redacted || i.safetyClass != other.safetyClass ||
		i.approvalDecision != other.approvalDecision || i.summary != other.summary ||
		i.droppedMessages != other.droppedMessages {
		return false
	}
	duration, known := optional.Present(i.executionDuration)
	otherDuration, otherKnown := optional.Present(other.executionDuration)
	failure, failed := optional.Present(i.failure)
	otherFailure, otherFailed := optional.Present(other.failure)
	if duration != otherDuration || known != otherKnown || failure != otherFailure || failed != otherFailed {
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
