// Package hooks owns the final fold of lifecycle advice. Portable rule values
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

// Fold combines one matching hook outcome using first-deny and first-rewrite
// precedence while accumulating injected context.
func (d *Decision) Fold(verdict Verdict, reason, inject, rewrite string) {
	switch {
	case verdict == VerdictBlock && d.Verdict != VerdictBlock:
		// A denial is final, so a rewrite an earlier hook asked for no longer
		// applies. Folding in a different order must not decide the call
		// differently.
		d.Verdict, d.Reason, d.RewriteArguments = VerdictBlock, reason, ""
	case verdict == VerdictAsk && d.Verdict != VerdictBlock:
		d.Verdict = VerdictAsk
		if d.Reason == "" {
			d.Reason = reason
		}
	}
	if inject != "" {
		if d.InjectContext != "" {
			d.InjectContext += "\n"
		}
		d.InjectContext += inject
	}
	if rewrite != "" && d.RewriteArguments == "" && d.Verdict != VerdictBlock {
		d.RewriteArguments = rewrite
	}
}
