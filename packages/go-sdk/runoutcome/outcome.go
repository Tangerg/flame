package runoutcome

type Kind string

const (
	Completed Kind = "completed"
	TimedOut  Kind = "timedOut"
	Failed    Kind = "failed"

	Canceled Kind = "canceled"
	Lost     Kind = "lost"
)
