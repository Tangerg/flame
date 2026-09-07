package render

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

type sessionSnapshotRecord struct {
	Session      protocol.Session   `json:"session"`
	Transcript   []blockFrame       `json:"transcript"`
	Runs         []runFrame         `json:"runs"`
	Plan         *planSnapshotFrame `json:"plan,omitempty"`
	Interactions []interactionJSON  `json:"interactions,omitempty"`
}

type runFrame struct {
	ID               string           `json:"id"`
	SessionID        string           `json:"sessionId"`
	SpawnedByBlockID string           `json:"spawnedByBlockId,omitempty"`
	ParentRunID      string           `json:"parentRunId,omitempty"`
	RootRunID        string           `json:"rootRunId,omitempty"`
	Provider         string           `json:"provider,omitempty"`
	Model            string           `json:"model,omitempty"`
	ReasoningEffort  string           `json:"reasoningEffort,omitempty"`
	Status           string           `json:"status"`
	ActiveSegmentID  string           `json:"activeSegmentId,omitempty"`
	CreatedAt        time.Time        `json:"createdAt,omitzero"`
	FinishedAt       time.Time        `json:"finishedAt,omitzero"`
	Limits           *runLimitsJSON   `json:"limits,omitempty"`
	ContextTokens    int64            `json:"contextTokens,omitempty"`
	Outcome          *outcomeJSON     `json:"outcome,omitempty"`
	Usage            usageJSON        `json:"usage"`
	ProtocolProfile  *runContractJSON `json:"protocolProfile,omitempty"`
}

type runLimitsJSON struct {
	MaxTotalTokens *int64   `json:"maxTotalTokens,omitempty"`
	MaxSteps       *int     `json:"maxSteps,omitempty"`
	MaxBudgetUSD   *float64 `json:"maxBudgetUsd,omitempty"`
}

type runContractJSON struct {
	RequiredFeatures []string `json:"requiredFeatures"`
	InterruptTypes   []string `json:"interruptTypes"`
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

type runPageRecord struct {
	Items      []runFrame `json:"items"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

type runCancellationRecord struct {
	Canceled runFrame `json:"canceled"`
	Root     runFrame `json:"root"`
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
		Runs:         make([]runFrame, 0, len(snapshot.Runs)),
		Plan:         encodePlanSnapshot(snapshot.Plan),
		Interactions: encodeInteractions(snapshot.Interactions),
	}
	for _, block := range snapshot.Transcript {
		record.Transcript = append(record.Transcript, *encodeBlock(block))
	}
	for _, run := range snapshot.Runs {
		record.Runs = append(record.Runs, encodeRun(run))
	}
	return json.NewEncoder(w).Encode(record)
}

func WriteRunJSON(w io.Writer, run agent.Run) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("render run: %w", err)
	}
	return json.NewEncoder(w).Encode(encodeRun(run))
}

func WriteRunPageJSON(w io.Writer, page agent.RunPage) error {
	if err := page.Validate(); err != nil {
		return fmt.Errorf("render run page: %w", err)
	}
	record := runPageRecord{Items: make([]runFrame, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, run := range page.Items {
		record.Items = append(record.Items, encodeRun(run))
	}
	return json.NewEncoder(w).Encode(record)
}

func WriteRunCancellationJSON(w io.Writer, result agent.RunCancellation) error {
	if err := result.Validate(); err != nil {
		return fmt.Errorf("render run cancellation: %w", err)
	}
	return json.NewEncoder(w).Encode(runCancellationRecord{
		Canceled: encodeRun(result.Canceled),
		Root:     encodeRun(result.Root),
	})
}

func encodeRun(run agent.Run) runFrame {
	encoded := runFrame{
		ID: run.ID, SessionID: run.SessionID,
		SpawnedByBlockID: run.Lineage.SpawnedByBlockID(),
		ParentRunID:      run.Lineage.ParentRunID(), RootRunID: run.Lineage.RootRunID(),
		Provider: run.Provider, Model: run.Model, ReasoningEffort: run.ReasoningEffort,
		Status: string(run.Status), ActiveSegmentID: run.ActiveSegmentID,
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
		ContextTokens: run.ContextTokens, Usage: *encodeUsage(run.Usage),
	}
	encoded.Limits = encodeRunLimits(run.Limits)
	if run.Status == protocol.RunStatusFinished {
		encoded.Outcome = encodeOutcome(run.Outcome)
	}
	if run.ProtocolProfile != nil {
		encoded.ProtocolProfile = &runContractJSON{
			RequiredFeatures: make([]string, len(run.ProtocolProfile.RequiredFeatures)),
			InterruptTypes:   make([]string, len(run.ProtocolProfile.InterruptTypes)),
		}
		for index, feature := range run.ProtocolProfile.RequiredFeatures {
			encoded.ProtocolProfile.RequiredFeatures[index] = string(feature)
		}
		for index, kind := range run.ProtocolProfile.InterruptTypes {
			encoded.ProtocolProfile.InterruptTypes[index] = string(kind)
		}
	}
	return encoded
}
