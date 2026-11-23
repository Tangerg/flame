package commit

type State uint8

const (
	Uncommitted State = iota
	Visible
	Durable
)

// A visible change must be adopted even when Err reports a later failure; retrying
// it as uncommitted could replace its identity.
type Result struct {
	State State
	Err   error
}

func (r Result) Committed() bool {
	return r.State == Visible || r.State == Durable
}
