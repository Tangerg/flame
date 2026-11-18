package runs

import "slices"

// Equal reports whether two Pending values describe the same durable tree
// hand-off. Empty collections compare equal whatever their backing, and the
// creation time compares by instant rather than location.
func (p Pending) Equal(other Pending) bool {
	return p.RootRunID == other.RootRunID &&
		p.ExecutorID == other.ExecutorID &&
		p.CreatedAt.Equal(other.CreatedAt) &&
		slices.EqualFunc(p.Interrupts, other.Interrupts, OpenInterrupt.Equal) &&
		slices.Equal(p.Bindings, other.Bindings) &&
		slices.EqualFunc(p.Continuations, other.Continuations, Continuation.Equal)
}

func (c Continuation) Equal(other Continuation) bool {
	return c.RunID == other.RunID && c.MemberID == other.MemberID && slices.Equal(c.DrainedTools, other.DrainedTools)
}

func (i OpenInterrupt) Equal(other OpenInterrupt) bool {
	if i.ItemID != other.ItemID || (i.Approval == nil) != (other.Approval == nil) {
		return false
	}
	return i.Approval == nil || *i.Approval == *other.Approval
}
