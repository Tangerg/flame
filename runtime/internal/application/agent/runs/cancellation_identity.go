package runs

import "github.com/google/uuid"

type CancellationID uuid.UUID

func newCancellationID() CancellationID { return CancellationID(uuid.New()) }

func (i CancellationID) String() string { return uuid.UUID(i).String() }
func (i CancellationID) IsZero() bool   { return i == CancellationID{} }

func ParseCancellationID(value string) (CancellationID, bool) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.String() != value {
		return CancellationID{}, false
	}
	return CancellationID(id), true
}
