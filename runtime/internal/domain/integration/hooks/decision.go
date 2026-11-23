package hooks

import "strings"

// Values are ordered by precedence: folding never lowers a verdict.
type Verdict int

const (
	VerdictAllow Verdict = iota
	VerdictAsk
	VerdictBlock
)

type Decision struct {
	Verdict          Verdict
	Reason           string
	InjectContext    string
	RewriteArguments string
}

// Whitespace contributes no context, so it neither reaches the model nor
// counts as a provenance source.
func (d Decision) InjectsContext() bool { return strings.TrimSpace(d.InjectContext) != "" }

// Then folds a later lifecycle decision into this one: the stricter verdict
// keeps its reason, and injected contexts accumulate in order.
func (d Decision) Then(next Decision) Decision {
	if next.Verdict > d.Verdict {
		d.Verdict, d.Reason = next.Verdict, next.Reason
	}
	if next.InjectsContext() {
		if d.InjectsContext() {
			d.InjectContext += "\n"
		}
		d.InjectContext += next.InjectContext
	}
	return d
}
