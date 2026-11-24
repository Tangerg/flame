package skill

type Scope string

const (
	Repository   Scope = "repository"
	User         Scope = "user"
	Installation Scope = "installation"
)

type ProposalOrigin string

const (
	Requested ProposalOrigin = "requested"
	Mined     ProposalOrigin = "mined"
)

const MaxPendingProposalsPerScope = 128
