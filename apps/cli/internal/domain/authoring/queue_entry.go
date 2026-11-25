package authoring

type QueueEntryID CommandID

func (i QueueEntryID) Validate() error { return CommandID(i).Validate() }
func (i QueueEntryID) String() string  { return string(i) }
