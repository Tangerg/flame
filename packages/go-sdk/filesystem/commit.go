package filesystem

type CommitState uint8

const (
	CommitUncommitted CommitState = iota
	CommitVisible
	CommitDurable
)

// CommitResult separates a visible filesystem change from its crash-durability
// confirmation. A visible change must be adopted even when Err reports a later
// failure; retrying it as an uncommitted operation could replace its identity.
type CommitResult struct {
	State CommitState
	Err   error
}

func (r CommitResult) Committed() bool {
	return r.State == CommitVisible || r.State == CommitDurable
}
