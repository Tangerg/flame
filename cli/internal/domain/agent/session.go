package agent

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/cli/internal/exactint"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

func validateCommittedRevision(owner string, value uint64) error {
	revision, err := exactint.Restore(value)
	if err != nil {
		return fmt.Errorf("%s revision: %w", owner, err)
	}
	if revision.IsZero() {
		return fmt.Errorf("%s revision must be positive", owner)
	}
	return nil
}

// ModelRef is one complete provider/model identity. Provider and model always
// move together so callers cannot create a session update that the Runtime has
// to guess how to complete.
type ModelRef struct {
	Provider string
	Model    string
}

const modelRefSeparator = "/"

func NewModelRef(provider, model string) (ModelRef, error) {
	ref := ModelRef{Provider: provider, Model: model}
	if err := ref.Validate(); err != nil {
		return ModelRef{}, err
	}
	return ref, nil
}

func ParseModelRef(value string) (ModelRef, error) {
	provider, model, found := strings.Cut(value, modelRefSeparator)
	if !found {
		return ModelRef{}, fmt.Errorf("model identity must use provider%smodel form", modelRefSeparator)
	}
	return NewModelRef(provider, model)
}

func (m ModelRef) Validate() error {
	if err := runtimeprotocol.ValidateModelSelection(m.Provider, m.Model, ""); err != nil {
		return err
	}
	if strings.Contains(m.Provider, modelRefSeparator) {
		return fmt.Errorf("model identity provider must not contain %q", modelRefSeparator)
	}
	return nil
}

func (m ModelRef) String() string { return m.Provider + modelRefSeparator + m.Model }

type SessionQuery struct {
	Cursor    string
	PageSize  PageSize
	Search    string
	Workspace string
}

// Normalize returns one exact session-catalog query. Search text and workspace
// input are presentation values; a non-empty workspace is an exact absolute
// identity because the Runtime binds it into the authoritative cursor query.
func (s SessionQuery) Normalize() (SessionQuery, error) {
	if _, err := s.PageSize.Rows(); err != nil {
		return SessionQuery{}, fmt.Errorf("session query: %w", err)
	}
	if !utf8.ValidString(s.Search) {
		return SessionQuery{}, errors.New("session query: search is not valid UTF-8")
	}
	s.Search = strings.TrimSpace(s.Search)
	if strings.ContainsRune(s.Search, 0) {
		return SessionQuery{}, errors.New("session query: search contains NUL")
	}
	if err := (runtimeprotocol.ListSessionsRequest{Search: s.Search}).ValidateWire(); err != nil {
		return SessionQuery{}, fmt.Errorf("session query: %w", err)
	}
	s.Workspace = strings.TrimSpace(s.Workspace)
	if err := (workspace.ResolveRequest{Path: s.Workspace}).Validate(); err != nil {
		return SessionQuery{}, fmt.Errorf("session query: %w", err)
	}
	if s.Workspace != "" && filepath.Clean(s.Workspace) != s.Workspace {
		return SessionQuery{}, errors.New("session query: workspace path is not canonical")
	}
	return s, nil
}

// SessionSnapshot is the cold-read projection the CLI restores. Transcript,
// Runs, Plan, and Goal are durable values, never reconstructed from a historical
// event stream. Runs contains roots and descendants in creation order.
type SessionSnapshot struct {
	Session      runtimeprotocol.Session
	Transcript   []Block
	Runs         []runtimeprotocol.RunRef
	Plan         *runtimeprotocol.Plan
	Goal         *runtimeprotocol.Goal
	Interactions []Interaction
}

// LatestRun returns the most recently created root run.
func (s SessionSnapshot) LatestRun() (runtimeprotocol.RunRef, bool) {
	for _, run := range slices.Backward(s.Runs) {
		if run.ParentRunID == "" {
			return CloneRun(run), true
		}
	}
	return runtimeprotocol.RunRef{}, false
}

// ActiveRun returns the sole running or waiting root run, when one exists.
func (s SessionSnapshot) ActiveRun() (runtimeprotocol.RunRef, bool) {
	for _, run := range slices.Backward(s.Runs) {
		if run.ParentRunID == "" && run.Status != runtimeprotocol.RunStatusFinished {
			return CloneRun(run), true
		}
	}
	return runtimeprotocol.RunRef{}, false
}

func (s SessionSnapshot) RunByID(id string) (runtimeprotocol.RunRef, bool) {
	for _, run := range s.Runs {
		if run.ID == id {
			return CloneRun(run), true
		}
	}
	return runtimeprotocol.RunRef{}, false
}

// LastAssistantText returns the latest durable non-empty assistant response.
func (s SessionSnapshot) LastAssistantText() (string, error) {
	for _, block := range slices.Backward(s.Transcript) {
		if block.Kind == BlockAssistant && strings.TrimSpace(block.Text) != "" {
			return block.Text, nil
		}
	}
	return "", errors.New("the session has no assistant response to copy")
}

func (c *Conversation) RestoreSnapshot(snapshot SessionSnapshot) {
	next := NewConversation()
	next.blocks = cloneBlocks(snapshot.Transcript)
	next.plan = clonePlan(snapshot.Plan)
	if next.plan != nil && next.plan.State != nil {
		next.restoredPlanRevision = next.plan.State.Revision
	}
	next.rebuildBlockIndex()
	for _, run := range snapshot.Runs {
		next.rememberRun(run)
	}
	if active, ok := snapshot.ActiveRun(); ok {
		next.runID = active.ID
		next.segmentID = active.ActiveSegmentID
		next.usage = UsageFromMetrics(active.Metrics)
		if active.Status == runtimeprotocol.RunStatusWaiting {
			next.phase = ConversationWaiting
			next.interactions = CloneInteractions(snapshot.Interactions)
		} else {
			next.phase = ConversationRunning
			next.coldTail = true
		}
	} else if latest, ok := snapshot.LatestRun(); ok {
		next.runID = latest.ID
		next.usage = UsageFromMetrics(latest.Metrics)
		next.outcome = OutcomeFromRun(latest.Outcome)
	}
	*c = *next
}

// RestoreAttachedSnapshot restores a cold projection that was read after a
// cursorless subscription was attached. HeadEventID is the exact journal
// position preceding that stream; retaining it closes a second-disconnect gap
// before the first replayable event arrives.
func (c *Conversation) RestoreAttachedSnapshot(snapshot SessionSnapshot, stream SegmentStream) error {
	if err := stream.ValidateSubscription(); err != nil {
		return fmt.Errorf("restore attached snapshot: %w", err)
	}
	active, ok := snapshot.ActiveRun()
	if !ok || active.Status != runtimeprotocol.RunStatusRunning {
		return errors.New("restore attached snapshot: snapshot has no running run")
	}
	if active.ID != stream.RunID || active.ActiveSegmentID != stream.SegmentID {
		return fmt.Errorf(
			"restore attached snapshot: stream %s/%s does not match run %s/%s",
			stream.RunID, stream.SegmentID, active.ID, active.ActiveSegmentID,
		)
	}
	c.RestoreSnapshot(snapshot)
	c.checkpoint = stream.HeadEventID
	c.reconciling = true
	return nil
}

type CreateSession struct {
	Title     string
	Workspace string
}

func (c CreateSession) Validate() error {
	if c.Title != "" && strings.TrimSpace(c.Title) == "" {
		return errors.New("session create: title is empty")
	}
	if err := (workspace.ResolveRequest{Path: strings.TrimSpace(c.Workspace)}).Validate(); err != nil {
		return fmt.Errorf("session create: %w", err)
	}
	return nil
}

type UpdateSession struct {
	SessionID        string
	Title            *string
	Workspace        *string
	Model            *ModelRef
	Favorite         *bool
	ExpectedRevision uint64
}

func (u UpdateSession) Validate() error {
	if err := runtimeprotocol.ValidateSessionID(u.SessionID); err != nil {
		return fmt.Errorf("session update: %w", err)
	}
	if u.Title == nil && u.Workspace == nil && u.Model == nil && u.Favorite == nil {
		return errors.New("session update: no fields are selected")
	}
	if u.Title != nil && strings.TrimSpace(*u.Title) == "" {
		return errors.New("session update: title is empty")
	}
	if u.Workspace != nil && strings.TrimSpace(*u.Workspace) == "" {
		return errors.New("session update: workspace is empty")
	}
	if u.Model != nil {
		if err := u.Model.Validate(); err != nil {
			return fmt.Errorf("session update: %w", err)
		}
	}
	if err := validateCommittedRevision("session update expected", u.ExpectedRevision); err != nil {
		return err
	}
	return nil
}

type ForkSession struct {
	SessionID string
	FromRunID string
	Title     string
}

func (f ForkSession) Validate() error {
	if err := runtimeprotocol.ValidateSessionID(f.SessionID); err != nil {
		return fmt.Errorf("session fork: %w", err)
	}
	if f.FromRunID != "" {
		if err := runtimeprotocol.ValidateRunID(f.FromRunID); err != nil {
			return fmt.Errorf("session fork: %w", err)
		}
	}
	if f.Title != "" && strings.TrimSpace(f.Title) == "" {
		return errors.New("session fork: title is empty")
	}
	return nil
}

// DeleteSession identifies one idempotent session-deletion intent. CommandID
// is optional for one-shot callers; durable clients set it so an interrupted
// acknowledgement can be recovered without issuing a second mutation.
type DeleteSession struct {
	CommandID CommandID
	SessionID string
}

func cloneBlocks(blocks []Block) []Block {
	out := make([]Block, len(blocks))
	for i, block := range blocks {
		out[i] = block.Clone()
	}
	return out
}
