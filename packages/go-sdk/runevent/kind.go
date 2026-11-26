package runevent

type Kind string

const (
	SegmentStarted  Kind = "segment.started"
	SegmentProgress Kind = "segment.progress"
	SegmentFinished Kind = "segment.finished"
	ItemStarted     Kind = "item.started"
	ItemDelta       Kind = "item.delta"
	ItemCompleted   Kind = "item.completed"
	PlanUpdated     Kind = "plan.updated"
)

type Reliability string

const (
	Authoritative Reliability = "authoritative"
	Ephemeral     Reliability = "ephemeral"
)

var catalog = [...]struct {
	kind        Kind
	reliability Reliability
}{
	{SegmentStarted, Authoritative},
	{SegmentProgress, Ephemeral},
	{SegmentFinished, Authoritative},
	{ItemStarted, Authoritative},
	{ItemDelta, Ephemeral},
	{ItemCompleted, Authoritative},
	{PlanUpdated, Authoritative},
}

func Kinds() []Kind {
	kinds := make([]Kind, 0, len(catalog))
	for _, entry := range catalog {
		kinds = append(kinds, entry.kind)
	}
	return kinds
}

func (Kind) Values() []string {
	values := make([]string, 0, len(catalog))
	for _, entry := range catalog {
		values = append(values, string(entry.kind))
	}
	return values
}

func (k Kind) Reliability() (Reliability, bool) {
	for _, entry := range catalog {
		if entry.kind == k {
			return entry.reliability, true
		}
	}
	return "", false
}

func (k Kind) Replayable() bool {
	reliability, _ := k.Reliability()
	return reliability == Authoritative
}

func (k Kind) Terminal() bool { return k == SegmentFinished }
