// Package commit describes physical publication evidence without performing I/O.
package commit

type State uint8

const (
	Uncommitted State = iota
	Visible
	Durable
)

// Result separates a visible filesystem change from its crash-durability
// confirmation. A visible change must be adopted even when Err reports a later
// failure; retrying it as an uncommitted operation could replace its identity.
type Result struct {
	State State
	Err   error
}

func (r Result) Committed() bool {
	return r.State == Visible || r.State == Durable
}
