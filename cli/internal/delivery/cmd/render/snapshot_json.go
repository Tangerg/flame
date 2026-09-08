package render

import (
	"encoding/json"
	"io"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

type sessionSnapshotRecord struct {
	Session      protocol.Session   `json:"session"`
	Transcript   []blockFrame       `json:"transcript"`
	Runs         []protocol.RunRef  `json:"runs"`
	Plan         *planSnapshotFrame `json:"plan,omitempty"`
	Interactions []interactionJSON  `json:"interactions,omitempty"`
}

type planSnapshotFrame struct {
	Revision uint64      `json:"revision"`
	Items    []planFrame `json:"items"`
}

func encodePlanSnapshot(plan *protocol.Plan) *planSnapshotFrame {
	if plan == nil || plan.State == nil {
		return nil
	}
	return &planSnapshotFrame{Revision: plan.State.Revision, Items: encodePlan(plan.State.Steps)}
}

// WriteSessionJSON writes one session using the same field contract as session
// pages and cold snapshots.
func WriteSessionJSON(w io.Writer, session protocol.Session) error {
	return json.NewEncoder(w).Encode(session)
}

func WriteSessionPageJSON(w io.Writer, page protocol.Page[protocol.Session]) error {
	return json.NewEncoder(w).Encode(page)
}

func WriteSessionSnapshotJSON(w io.Writer, snapshot agent.SessionSnapshot) error {
	record := sessionSnapshotRecord{
		Session:      snapshot.Session,
		Transcript:   make([]blockFrame, 0, len(snapshot.Transcript)),
		Runs:         snapshot.Runs,
		Plan:         encodePlanSnapshot(snapshot.Plan),
		Interactions: encodeInteractions(snapshot.Interactions),
	}
	for _, block := range snapshot.Transcript {
		record.Transcript = append(record.Transcript, *encodeBlock(block))
	}
	return json.NewEncoder(w).Encode(record)
}

func WriteRunJSON(w io.Writer, run protocol.RunRef) error { return json.NewEncoder(w).Encode(run) }

func WriteRunPageJSON(w io.Writer, page protocol.Page[protocol.RunRef]) error {
	return json.NewEncoder(w).Encode(page)
}

func WriteRunCancellationJSON(w io.Writer, result protocol.CancelRunResponse) error {
	return json.NewEncoder(w).Encode(result)
}
