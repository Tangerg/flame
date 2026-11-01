package skill

type Scope string

const (
	Project      Scope = "project"
	User         Scope = "user"
	Installation Scope = "installation"
)

type ProposalOrigin string

const (
	Requested ProposalOrigin = "requested"
	Mined     ProposalOrigin = "mined"
)

// A library's review queue is returned in one bounded read.
const MaxPendingProposalsPerScope = 128
