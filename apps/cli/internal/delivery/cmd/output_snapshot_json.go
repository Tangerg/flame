package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type sessionSnapshotRecord struct {
	Session    sessionFrame       `json:"session"`
	Transcript []blockFrame       `json:"transcript"`
	Runs       []runJSONFrame     `json:"runs"`
	Plan       *planSnapshotFrame `json:"plan,omitzero"`
	Interrupts []interruptJSON    `json:"interrupts,omitempty"`
}

type sessionPageRecord struct {
	Items      []sessionFrame `json:"items"`
	NextCursor string         `json:"nextCursor,omitzero"`
}

type sessionFrame struct {
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	Status          string         `json:"status"`
	Provider        string         `json:"provider"`
	Model           string         `json:"model,omitzero"`
	ReasoningEffort string         `json:"reasoningEffort,omitzero"`
	Workspace       workspaceFrame `json:"workspace"`
	CreatedAt       time.Time      `json:"createdAt,omitzero"`
	UpdatedAt       time.Time      `json:"updatedAt,omitzero"`
	Favorite        bool           `json:"favorite,omitzero"`
	Revision        uint64         `json:"revision"`
}

type workspaceFrame struct {
	Path         string `json:"path"`
	ProjectRoot  string `json:"projectRoot"`
	Availability string `json:"availability"`
}

type runJSONFrame struct {
	ID               string    `json:"id"`
	SessionID        string    `json:"sessionId"`
	SpawnedByBlockID string    `json:"spawnedByBlockId,omitzero"`
	ParentRunID      string    `json:"parentRunId,omitzero"`
	RootRunID        string    `json:"rootRunId,omitzero"`
	Provider         string    `json:"provider,omitzero"`
	Model            string    `json:"model,omitzero"`
	ReasoningEffort  string    `json:"reasoningEffort,omitzero"`
	Status           string    `json:"status"`
	ActiveSegmentID  string    `json:"activeSegmentId,omitzero"`
	CreatedAt        time.Time `json:"createdAt,omitzero"`
	FinishedAt       time.Time `json:"finishedAt,omitzero"`

	ContextTokens   int64            `json:"contextTokens,omitzero"`
	Outcome         *outcomeJSON     `json:"outcome,omitzero"`
	Usage           usageJSON        `json:"usage"`
	ProtocolProfile *runContractJSON `json:"protocolProfile,omitzero"`
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
	Items      []runJSONFrame `json:"items"`
	NextCursor string         `json:"nextCursor,omitzero"`
}

type runCancellationRecord struct {
	Canceled runJSONFrame `json:"canceled"`
	Root     runJSONFrame `json:"root"`
}

func writeSessionJSON(w io.Writer, session conversation.Session) error {
	return writeJSONLine(w, encodeSession(session))
}

func writeSessionPageJSON(w io.Writer, page conversation.SessionPage) error {
	record := sessionPageRecord{Items: make([]sessionFrame, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, session := range page.Items {
		record.Items = append(record.Items, encodeSession(session))
	}
	return writeJSONLine(w, record)
}

// Domain values intentionally carry no encoding tags, so this adapter owns the
// external field names instead of leaking a delivery format into the core.
func writeSessionSnapshotJSON(w io.Writer, snapshot conversation.SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("render session snapshot: %w", err)
	}
	record := sessionSnapshotRecord{
		Session:    encodeSession(snapshot.Session),
		Transcript: make([]blockFrame, 0, len(snapshot.Transcript)),
		Runs:       make([]runJSONFrame, 0, len(snapshot.Runs)),
		Plan:       encodePlanSnapshot(snapshot.Plan),
		Interrupts: encodeInterrupts(snapshot.Interrupts),
	}
	for _, block := range snapshot.Transcript {
		record.Transcript = append(record.Transcript, *encodeBlock(block))
	}
	for _, run := range snapshot.Runs {
		record.Runs = append(record.Runs, encodeRun(run))
	}
	return writeJSONLine(w, record)
}

func encodeSession(session conversation.Session) sessionFrame {
	return sessionFrame{
		ID: session.ID, Title: session.Title, Status: string(session.Status),
		Provider: session.Provider, Model: session.Model, ReasoningEffort: session.ReasoningEffort,
		Workspace: workspaceFrame{
			Path: session.Workspace.Path, ProjectRoot: session.Workspace.ProjectRoot,
			Availability: string(session.Workspace.Availability),
		},
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
		Favorite: session.Favorite, Revision: session.Revision,
	}
}

func writeRunJSON(w io.Writer, run conversation.Run) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("render run: %w", err)
	}
	return writeJSONLine(w, encodeRun(run))
}

func writeRunPageJSON(w io.Writer, page conversation.RunPage) error {
	if err := page.Validate(); err != nil {
		return fmt.Errorf("render run page: %w", err)
	}
	record := runPageRecord{Items: make([]runJSONFrame, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, run := range page.Items {
		record.Items = append(record.Items, encodeRun(run))
	}
	return writeJSONLine(w, record)
}

func writeRunCancellationJSON(w io.Writer, result conversation.RunCancellation) error {
	if err := result.Validate(); err != nil {
		return fmt.Errorf("render run cancellation: %w", err)
	}
	return writeJSONLine(w, runCancellationRecord{
		Canceled: encodeRun(result.Canceled),
		Root:     encodeRun(result.Root),
	})
}

func encodeRun(run conversation.Run) runJSONFrame {
	encoded := runJSONFrame{
		ID: run.ID, SessionID: run.SessionID,
		SpawnedByBlockID: run.Lineage.SpawnedByBlockID(),
		ParentRunID:      run.Lineage.ParentRunID(), RootRunID: run.Lineage.RootRunID(),
		Provider: run.Provider, Model: run.Model, ReasoningEffort: run.ReasoningEffort,
		Status: string(run.Status), ActiveSegmentID: run.ActiveSegmentID,
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
		ContextTokens: run.ContextTokens, Usage: *encodeUsage(run.Usage),
	}
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
