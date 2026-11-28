package persistence

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/scope/core/chat"
)

type forkHistory struct {
	Child          forkChild
	Messages       []chat.Message
	Runs           []runRow
	Items          []storedTranscriptRow
	ToolResults    []transcript.ToolResultBlob
	PlanBoundaries []sessions.RunPlanBoundary
	InitialPlan    *plan.Snapshot
}

type forkChild struct {
	ID        identity.SessionID
	ParentID  identity.SessionID
	Project   identity.ProjectID
	Title     string
	Selection flamemodel.Selection
	CreatedAt time.Time
	UpdatedAt time.Time
	Favorite  bool
	Isolated  bool
	Revision  uint64
}

func encodeForkHistory(fork sessions.ForkPlan) ([]byte, error) {
	history := fork.History()
	child := history.Session.Snapshot()
	staged := forkHistory{
		Child: forkChild{
			ID: child.ID, ParentID: child.ParentID, Project: child.Project, Title: child.Title,
			Selection: child.Selection, CreatedAt: child.CreatedAt, UpdatedAt: child.UpdatedAt,
			Favorite: child.Favorite, Isolated: child.Isolated, Revision: child.Revision,
		},
		Messages: history.Messages, ToolResults: history.ToolResults, PlanBoundaries: fork.PlanBoundaries(),
	}
	for _, value := range history.Runs {
		stored, err := encodeTerminalRun(value)
		if err != nil {
			return nil, err
		}
		staged.Runs = append(staged.Runs, stored)
	}
	for _, item := range history.Items {
		stored, err := encodeStoredTranscriptRow(item)
		if err != nil {
			return nil, err
		}
		staged.Items = append(staged.Items, stored)
	}
	if replacement := fork.PlanReplacement(); replacement != nil {
		staged.InitialPlan = new(replacement.State().Snapshot())
	}
	return encodeStoredJSON(staged)
}

func (f forkHistory) restore(checkout string) (sessions.HistorySnapshot, *plan.Replacement, error) {
	workspace, err := session.NewWorkspace(checkout)
	if err != nil {
		return sessions.HistorySnapshot{}, nil, err
	}
	child, err := session.Restore(session.Snapshot{
		ID: f.Child.ID, ParentID: f.Child.ParentID, Project: f.Child.Project, Title: f.Child.Title,
		Workspace: workspace, Selection: f.Child.Selection, Favorite: f.Child.Favorite, Isolated: f.Child.Isolated,
		CreatedAt: f.Child.CreatedAt, UpdatedAt: f.Child.UpdatedAt, Revision: f.Child.Revision,
	})
	if err != nil {
		return sessions.HistorySnapshot{}, nil, err
	}
	history := sessions.HistorySnapshot{Session: child, Messages: f.Messages, ToolResults: f.ToolResults}
	for _, stored := range f.Runs {
		snapshot, err := stored.snapshot()
		if err != nil {
			return sessions.HistorySnapshot{}, nil, err
		}
		value, err := run.Restore(snapshot)
		if err != nil {
			return sessions.HistorySnapshot{}, nil, err
		}
		history.Runs = append(history.Runs, value)
	}
	for _, item := range f.Items {
		snapshot, err := item.snapshot()
		if err != nil {
			return sessions.HistorySnapshot{}, nil, err
		}
		value, err := transcript.RestoreItem(snapshot)
		if err != nil {
			return sessions.HistorySnapshot{}, nil, err
		}
		history.Items = append(history.Items, value)
	}
	if err := history.ValidateToolResults(); err != nil {
		return sessions.HistorySnapshot{}, nil, err
	}
	if f.InitialPlan == nil {
		return history, nil, nil
	}
	state, err := plan.Restore(*f.InitialPlan)
	if err != nil {
		return sessions.HistorySnapshot{}, nil, err
	}
	replacement, err := plan.NewReplacement(plan.Current{}.Version(), state)
	return history, &replacement, err
}
