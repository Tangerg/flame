package workbench

// CommandPhase records whether the Runtime has acknowledged a staged command.
// A prepared command may still be replayed; a confirmed one is only reconciled.
type CommandPhase string

const (
	CommandPrepared  CommandPhase = "prepared"
	CommandConfirmed CommandPhase = "confirmed"
)
