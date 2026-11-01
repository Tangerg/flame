// Package hooks owns Runtime adoption of lifecycle advice. Portable rule values
// and command material are defined by the public hook contract.
package hooks

// Verdict is the control outcome of the hooks that fired for one event. The
// values are ordered by precedence: folding never lowers a verdict.
type Verdict int

const (
	// VerdictAllow lets the action proceed.
	VerdictAllow Verdict = iota
	// VerdictAsk forces an approval prompt for a PreToolUse the gate would
	// otherwise pass (a hook escalating to human review).
	VerdictAsk
	// VerdictBlock denies the action (the tool, or the prompt).
	VerdictBlock
)

// Decision is the combined verdict of every hook that fired for one event.
type Decision struct {
	Verdict Verdict
	// Reason explains a block or an escalation; a block's is fed to the model so
	// it knows why.
	Reason string
	// InjectContext is extra context to surface (concatenated across hooks).
	InjectContext string
	// RewriteArguments, when set (PreToolUse), replaces the tool's arguments
	// (raw JSON). First non-empty wins.
	RewriteArguments string
}
