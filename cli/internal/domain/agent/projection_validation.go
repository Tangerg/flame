package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// Validate checks the CLI-owned construction of a Run projection. Runtime owns
// every wire fact the adapter copied in, so only the closed values this package
// builds itself are checked here; a zero Run is the state worth rejecting.
func (r Run) Validate() error {
	var problems []error
	if err := r.Lineage.validate(r.ID); err != nil {
		problems = append(problems, err)
	}
	if err := r.Limits.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

func (r RunLineage) validate(runID string) error {
	switch r.kind {
	case rootRunLineage:
		if r.spawnedByBlockID != "" || r.parentRunID != "" || r.rootRunID != "" {
			return errors.New("root run lineage carries child identity")
		}
		return nil
	case childRunLineage:
		if err := runtimeprotocol.ValidateRunID(runID); err != nil {
			return fmt.Errorf("child run lineage: %w", err)
		}
		if err := runtimeprotocol.ValidateItemID(r.spawnedByBlockID); err != nil {
			return fmt.Errorf("child run lineage spawn block: %w", err)
		}
		if err := runtimeprotocol.ValidateRunID(r.parentRunID); err != nil {
			return fmt.Errorf("child run lineage parent: %w", err)
		}
		if err := runtimeprotocol.ValidateRunID(r.rootRunID); err != nil {
			return fmt.Errorf("child run lineage root: %w", err)
		}
	case 0:
		return errors.New("run lineage is not initialized")
	default:
		return errors.New("run lineage kind is unknown")
	}
	switch {
	case r.parentRunID == runID:
		return errors.New("run lineage names itself as parent")
	case r.rootRunID == runID:
		return errors.New("run lineage names itself as root")
	default:
		return nil
	}
}

// Validate checks authored run options before they reach the Runtime. These
// values come from CLI flags, preferences and terminal forms, so they are the
// untrusted input this boundary owns.
func (r RunOptions) Validate() error {
	var problems []error
	if err := runtimeprotocol.ValidateModelSelection(r.Provider, r.Model, r.ReasoningEffort); err != nil {
		problems = append(problems, err)
	}
	if err := r.Limits.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := r.Generation.ValidateWire(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run options: %w", err)
	}
	return nil
}

// ValidateEvent checks the CLI projection of one runtime event before the
// conversation folds it. Wire shape belongs to Runtime; what remains here is
// the CLI's own block vocabulary and the folding facts it depends on.
func ValidateEvent(event Event) error {
	switch item := event.(type) {
	case SegmentStarted:
		if err := item.Run.Validate(); err != nil {
			return fmt.Errorf("segment started: %w", err)
		}
		if item.Run.Status != runtimeprotocol.RunStatusRunning {
			return errors.New("segment started with a non-running run")
		}
		return nil
	case BlockStarted:
		return item.Block.validateLifecycle(false)
	case BlockDelta:
		if item.Text == "" {
			return errors.New("block delta without text")
		}
		return nil
	case ToolArgumentsDelta, RunProgress, RunInterrupted, RunSuspended:
		return nil
	case CustomEvent:
		if strings.TrimSpace(item.Name) == "" {
			return errors.New("custom event without a name")
		}
		if !json.Valid(item.PayloadJSON) {
			return errors.New("custom event payload is not valid JSON")
		}
		return nil
	case BlockCompleted:
		return item.Block.validateLifecycle(true)
	case PlanChanged:
		_, err := committedPlanState(&item.Plan)
		return err
	case RunFinished:
		return nil
	case nil:
		return errors.New("event is nil")
	default:
		return fmt.Errorf("event %T is unsupported", event)
	}
}

func (b Block) validateLifecycle(completed bool) error {
	if err := b.validateEnvelope(completed); err != nil {
		return err
	}
	if err := b.validateAttachments(); err != nil {
		return err
	}
	return b.validateProjection()
}

// validateEnvelope checks the block vocabulary this package derives from a
// Runtime Item: kind, status, and which projections each kind may carry.
func (b Block) validateEnvelope(completed bool) error {
	if !slices.Contains([]BlockStatus{BlockStatusRunning, BlockStatusCompleted, BlockStatusIncomplete}, b.Status) {
		return fmt.Errorf("block %s has invalid status %q", b.ID, b.Status)
	}
	if completed == (b.Status == BlockStatusRunning) {
		return fmt.Errorf("block %s status %q disagrees with its event lifecycle", b.ID, b.Status)
	}
	if !slices.Contains([]BlockKind{BlockUser, BlockAssistant, BlockReasoning, BlockQuestion, BlockTool, BlockNotice, BlockError}, b.Kind) {
		return fmt.Errorf("block %s has invalid kind %q", b.ID, b.Kind)
	}
	if b.Status == BlockStatusRunning && !slices.Contains([]BlockKind{BlockAssistant, BlockReasoning, BlockTool}, b.Kind) {
		return fmt.Errorf("%s block %s cannot be running", b.Kind, b.ID)
	}
	wholeOnly := slices.Contains([]BlockKind{BlockUser, BlockQuestion, BlockNotice}, b.Kind)
	if wholeOnly && b.Status != BlockStatusCompleted {
		return fmt.Errorf("%s block %s must be completed", b.Kind, b.ID)
	}
	if b.Kind != BlockUser && len(b.Attachments) != 0 {
		return fmt.Errorf("%s block %s carries attachments", b.Kind, b.ID)
	}
	if b.Kind != BlockAssistant && len(b.Images) != 0 {
		return fmt.Errorf("%s block %s carries inline images", b.Kind, b.ID)
	}
	if b.Kind == BlockTool && !b.CreatedAt.IsZero() {
		return fmt.Errorf("tool block %s carries a message creation time", b.ID)
	}
	if b.Kind != BlockReasoning && b.Redacted {
		return fmt.Errorf("%s block %s is marked as redacted reasoning", b.Kind, b.ID)
	}
	if b.Kind != BlockNotice && b.DroppedMessages != 0 {
		return fmt.Errorf("%s block %s carries a dropped-message count", b.Kind, b.ID)
	}
	return nil
}

func (b Block) validateAttachments() error {
	for i, attachment := range b.Attachments {
		if err := attachment.Validate(); err != nil {
			return fmt.Errorf("block %s attachment %d: %w", b.ID, i+1, err)
		}
	}
	for i, image := range b.Images {
		if err := image.Validate(); err != nil {
			return fmt.Errorf("block %s inline image %d: %w", b.ID, i+1, err)
		}
	}
	return nil
}

// Validate checks an inline image the CLI decoded from Runtime content. Name
// and MIME type are derived here, so their presentation contract is CLI-owned.
func (i InlineImage) Validate() error {
	var problems []error
	if strings.TrimSpace(i.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(i.Name) == "" {
		problems = append(problems, errors.New("name is empty"))
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(i.MIMEType)), "image/") {
		problems = append(problems, errors.New("MIME type is not an image"))
	}
	if len(i.Data) == 0 {
		problems = append(problems, errors.New("data is empty"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("inline image: %w", err)
	}
	return nil
}

func (b Block) validateProjection() error {
	switch b.Kind {
	case BlockQuestion:
		return b.validateQuestionProjection()
	case BlockTool:
		return b.validateToolProjection()
	default:
		if b.Question != nil {
			return fmt.Errorf("%s block %s carries a question projection", b.Kind, b.ID)
		}
		if b.Tool != nil {
			return fmt.Errorf("%s block %s carries a tool projection", b.Kind, b.ID)
		}
	}
	return nil
}

func (b Block) validateQuestionProjection() error {
	if b.Question == nil {
		return fmt.Errorf("question block %s has no question projection", b.ID)
	}
	if b.Question.ItemID != b.ID {
		return fmt.Errorf("question block %s carries item id %s", b.ID, b.Question.ItemID)
	}
	if err := b.Question.Validate(); err != nil {
		return fmt.Errorf("block %s: %w", b.ID, err)
	}
	return nil
}

func (b Block) validateToolProjection() error {
	if b.Tool == nil {
		return fmt.Errorf("tool block %s has no tool projection", b.ID)
	}
	if err := b.Tool.Validate(); err != nil {
		return fmt.Errorf("block %s: %w", b.ID, err)
	}
	want := b.Tool.Status.blockStatus()
	if b.Status != want {
		return fmt.Errorf("block %s is %s while its tool is %s", b.ID, b.Status, b.Tool.Status)
	}
	return nil
}

func (t ToolStatus) blockStatus() BlockStatus {
	switch t {
	case ToolRunning:
		return BlockStatusRunning
	case ToolOK:
		return BlockStatusCompleted
	default:
		return BlockStatusIncomplete
	}
}

// validateUsageProgress protects the conversation fold: cumulative Run metering
// may only move forward across the events and snapshots the CLI merges.
func validateUsageProgress(previous, next Usage) error {
	if err := validateModelUsageProgress("total", runtimeprotocol.ModelUsage{
		InputTokens: previous.InputTokens, OutputTokens: previous.OutputTokens,
		CacheReadTokens: previous.CacheReadTokens, CacheWriteTokens: previous.CacheWriteTokens,
		ReasoningTokens: previous.ReasoningTokens, CostUSD: previous.CostUSD,
	}, runtimeprotocol.ModelUsage{
		InputTokens: next.InputTokens, OutputTokens: next.OutputTokens,
		CacheReadTokens: next.CacheReadTokens, CacheWriteTokens: next.CacheWriteTokens,
		ReasoningTokens: next.ReasoningTokens, CostUSD: next.CostUSD,
	}); err != nil {
		return err
	}
	switch {
	case next.Steps < previous.Steps:
		return errors.New("step usage regressed")
	case next.Duration < previous.Duration:
		return errors.New("active duration regressed")
	}
	for model, previousUsage := range previous.ByModel {
		nextUsage, exists := next.ByModel[model]
		if !exists {
			return fmt.Errorf("model usage %q disappeared", model)
		}
		if err := validateModelUsageProgress("model "+model, previousUsage, nextUsage); err != nil {
			return err
		}
	}
	return nil
}

func validateModelUsageProgress(label string, previous, next runtimeprotocol.ModelUsage) error {
	switch {
	case next.InputTokens < previous.InputTokens:
		return fmt.Errorf("%s input-token usage regressed", label)
	case next.OutputTokens < previous.OutputTokens:
		return fmt.Errorf("%s output-token usage regressed", label)
	case next.CacheReadTokens < previous.CacheReadTokens:
		return fmt.Errorf("%s cache-read usage regressed", label)
	case next.CacheWriteTokens < previous.CacheWriteTokens:
		return fmt.Errorf("%s cache-write usage regressed", label)
	case next.ReasoningTokens < previous.ReasoningTokens:
		return fmt.Errorf("%s reasoning-token usage regressed", label)
	case previous.CostUSD != nil && next.CostUSD != nil && *next.CostUSD < *previous.CostUSD:
		return fmt.Errorf("%s cost usage regressed", label)
	default:
		return nil
	}
}

// validateInteractionItem binds one pending interaction to the transcript block
// the CLI folded for it. The two projections come from different reads, so this
// relationship is established here and nowhere else.
func validateInteractionItem(interaction Interaction, block Block) error {
	itemID := InteractionItemID(interaction)
	if block.ID != itemID {
		return fmt.Errorf("interaction item %s resolved to block %s", itemID, block.ID)
	}
	if runID := InteractionRunID(interaction); block.RunID != runID {
		return fmt.Errorf("interaction item %s belongs to run %s, not %s", itemID, runID, block.RunID)
	}
	switch item := interaction.(type) {
	case Approval:
		if block.Kind != BlockTool || block.Status != BlockStatusRunning || block.Tool == nil {
			return fmt.Errorf("approval item %s is not a running tool", itemID)
		}
		if !block.Tool.sameInvocation(*item.Tool) {
			return fmt.Errorf("approval item %s differs from its tool block", itemID)
		}
	case Question:
		if block.Kind != BlockQuestion || block.Status != BlockStatusCompleted || block.Question == nil {
			return fmt.Errorf("question item %s is not a completed question", itemID)
		}
		if !block.Question.Equal(item) {
			return fmt.Errorf("question item %s differs from its question block", itemID)
		}
	default:
		return fmt.Errorf("interaction item %s has unsupported type %T", itemID, interaction)
	}
	return nil
}
