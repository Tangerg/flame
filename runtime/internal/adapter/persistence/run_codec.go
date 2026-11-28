package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

type runAccountingRow struct {
	Reported         *runUsageRow `json:"reported,omitzero"`
	UnreportedModels []string     `json:"unreportedModels,omitempty"`
}

type runUsageRow struct {
	InputTokens      int64                     `json:"inputTokens,omitzero"`
	OutputTokens     int64                     `json:"outputTokens,omitzero"`
	CacheReadTokens  int64                     `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64                     `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64                     `json:"reasoningTokens,omitzero"`
	CostNanoUSD      *int64                    `json:"costNanoUsd,omitzero"`
	ByModel          map[string]runModelRowUse `json:"byModel,omitempty"`
}

type runModelRowUse struct {
	InputTokens      int64  `json:"inputTokens,omitzero"`
	OutputTokens     int64  `json:"outputTokens,omitzero"`
	CacheReadTokens  int64  `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64  `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64  `json:"reasoningTokens,omitzero"`
	CostNanoUSD      *int64 `json:"costNanoUsd,omitzero"`
}

// Interrupt types are stored by canonical name, not ordinal, so inserting a type
// cannot re-label stored rows.
type runCapabilitiesRow struct {
	ChildRuns      bool                      `json:"childRuns,omitzero"`
	InterruptTypes []rundomain.InterruptType `json:"interruptTypes,omitempty"`
}

func encodeRunCapabilities(capabilities rundomain.Capabilities) (string, error) {
	if err := capabilities.Validate(); err != nil {
		return "", fmt.Errorf("encode run capabilities: %w", err)
	}
	if capabilities.IsEmpty() {
		return "", nil
	}
	row := runCapabilitiesRow{ChildRuns: capabilities.ChildRuns}
	row.InterruptTypes = append(row.InterruptTypes, capabilities.InterruptTypes...)
	encoded, err := encodeStoredJSON(row)
	if err != nil {
		return "", fmt.Errorf("encode run capabilities: %w", err)
	}
	return string(encoded), nil
}

func decodeRunCapabilities(encoded string) (rundomain.Capabilities, error) {
	if encoded == "" {
		return rundomain.Capabilities{}, nil
	}
	var row runCapabilitiesRow
	if err := decodeStoredJSON([]byte(encoded), &row); err != nil {
		return rundomain.Capabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	capabilities := rundomain.Capabilities{ChildRuns: row.ChildRuns}
	for _, kind := range row.InterruptTypes {
		if !kind.Valid() {
			// A stored kind this build cannot raise would let the Run park on
			// something nothing answers. Refusing the row is the honest outcome.
			return rundomain.Capabilities{}, fmt.Errorf("decode run capabilities: unknown interrupt kind %q", kind)
		}
		capabilities.InterruptTypes = append(capabilities.InterruptTypes, kind)
	}
	if err := capabilities.Validate(); err != nil {
		return rundomain.Capabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	if capabilities.IsEmpty() {
		return rundomain.Capabilities{}, errors.New("decode run capabilities: empty capabilities must use the absent value")
	}
	return capabilities, nil
}

type runProblemRow struct {
	Kind              rundomain.FailureKind `json:"kind"`
	Detail            string                `json:"detail,omitempty"`
	DocURL            string                `json:"docUrl,omitempty"`
	RetryAfterSeconds int                   `json:"retryAfterSeconds,omitzero"`
}

type metricsValues struct {
	steps      int
	durationNs int64
	usage      string
}

func runMetricsRow(metrics rundomain.Metrics) (metricsValues, error) {
	reported, ok := metrics.Usage()
	var usage *accounting.Usage
	if ok {
		usage = &reported
	}
	encoded, err := encodeRunUsage(usage, metrics.UnreportedModels())
	if err != nil {
		return metricsValues{}, err
	}
	return metricsValues{
		steps:      metrics.Steps(),
		durationNs: int64(metrics.ActiveDuration()),
		usage:      encoded,
	}, nil
}

func encodeRunUsage(usage *accounting.Usage, models []string) (string, error) {
	row := runAccountingRow{Reported: runUsageRowOf(usage), UnreportedModels: models}
	if row.Reported == nil && len(models) == 0 {
		return "", nil
	}
	encoded, err := encodeStoredJSON(row)
	if err != nil {
		return "", fmt.Errorf("encode run usage: %w", err)
	}
	return string(encoded), nil
}

func runUsageRowOf(usage *accounting.Usage) *runUsageRow {
	if usage == nil {
		return nil
	}
	total, byModel := usage.Total(), usage.ByModel()
	row := &runUsageRow{
		InputTokens:      total.InputTokens,
		OutputTokens:     total.OutputTokens,
		CacheReadTokens:  total.CacheReadTokens,
		CacheWriteTokens: total.CacheWriteTokens,
		ReasoningTokens:  total.ReasoningTokens,
		CostNanoUSD:      total.Cost.OptionalNanoUSD(),
	}
	if len(byModel) > 0 {
		row.ByModel = make(map[string]runModelRowUse, len(byModel))
		for model, perModel := range byModel {
			row.ByModel[model] = runModelRowUse{
				InputTokens:      perModel.InputTokens,
				OutputTokens:     perModel.OutputTokens,
				CacheReadTokens:  perModel.CacheReadTokens,
				CacheWriteTokens: perModel.CacheWriteTokens,
				ReasoningTokens:  perModel.ReasoningTokens,
				CostNanoUSD:      perModel.Cost.OptionalNanoUSD(),
			}
		}
	}
	return row
}

type runEndingDetailRow struct {
	Detail string `json:"detail,omitempty"`
}

type runFailureRow struct {
	Failure *runProblemRow `json:"failure"`
}

type runLossRow struct {
	Loss *runLossEvidenceRow `json:"loss"`
}

type runLossEvidenceRow struct {
	Reason rundomain.LossReason `json:"reason"`
	Detail string               `json:"detail,omitempty"`
}

func encodeRunEnding(ending rundomain.Ending) (outcome, encoded string, err error) {
	var row any
	switch ending := ending.(type) {
	case rundomain.Completed:
		return string(rundomain.OutcomeCompleted), "", nil
	case rundomain.Canceled:
		row = runEndingDetailRow{Detail: ending.Detail}
	case rundomain.TimedOut:
		row = runEndingDetailRow{Detail: ending.Detail}
	case rundomain.Failed:
		row = runFailureRow{Failure: &runProblemRow{
			Kind: ending.Failure.Kind, Detail: ending.Failure.Detail, DocURL: ending.Failure.DocURL,
			RetryAfterSeconds: ending.Failure.RetryAfterSeconds(),
		}}
	case rundomain.Lost:
		row = runLossRow{Loss: &runLossEvidenceRow{Reason: ending.Loss.Reason, Detail: ending.Loss.Detail}}
	default:
		return "", "", fmt.Errorf("encode run ending: unknown ending %T", ending)
	}
	data, err := encodeStoredJSON(row)
	if err != nil {
		return "", "", fmt.Errorf("encode run ending: %w", err)
	}
	return string(ending.Outcome()), string(data), nil
}

func decodeRunEnding(outcome rundomain.Outcome, encoded string) (rundomain.Ending, error) {
	switch outcome {
	case rundomain.OutcomeCompleted:
		if encoded != "" {
			return nil, errors.New("a completed Run stores ending facts")
		}
		return rundomain.Completed{}, nil
	case rundomain.OutcomeCanceled, rundomain.OutcomeTimedOut:
		var row *runEndingDetailRow
		if err := decodeStoredJSON([]byte(encoded), &row); err != nil {
			return nil, err
		}
		if row == nil {
			return nil, errors.New("Run ending detail must be an object")
		}
		if outcome == rundomain.OutcomeCanceled {
			return rundomain.Canceled{Detail: row.Detail}, nil
		}
		return rundomain.TimedOut{Detail: row.Detail}, nil
	case rundomain.OutcomeFailed:
		var row runFailureRow
		if err := decodeStoredJSON([]byte(encoded), &row); err != nil {
			return nil, err
		}
		if row.Failure == nil {
			return nil, errors.New("a failed Run stores no failure")
		}
		failure, err := row.Failure.failure()
		if err != nil {
			return nil, err
		}
		return rundomain.Failed{Failure: failure}, nil
	case rundomain.OutcomeLost:
		var row runLossRow
		if err := decodeStoredJSON([]byte(encoded), &row); err != nil {
			return nil, err
		}
		if row.Loss == nil {
			return nil, errors.New("a lost Run stores no loss")
		}
		return rundomain.Lost{Loss: rundomain.Loss{Reason: row.Loss.Reason, Detail: row.Loss.Detail}}, nil
	default:
		return nil, fmt.Errorf("unknown outcome %q", outcome)
	}
}

// Boot recovery is the one reader that may see a Waiting Run without its pending
// set: it must terminalize a row whose set was lost in the crash.
type pendingReadPolicy uint8

const (
	requirePendingSet pendingReadPolicy = iota
	allowMissingPendingSet
)

func scanRun(row scanRow) (rundomain.Run, error) {
	return scanRunRow(row, requirePendingSet)
}

func scanRunForRecovery(row scanRow) (rundomain.Run, error) {
	return scanRunRow(row, allowMissingPendingSet)
}

func scanRunRow(row scanRow, pendingPolicy pendingReadPolicy) (rundomain.Run, error) {
	var r runRow
	if err := row.Scan(
		&r.ID, &r.SessionID,
		&r.SpawnedByItemID, &r.ParentRunID, &r.RootRunID,
		&r.ActiveSegmentID, &r.Outcome,
		&r.Provider, &r.Model, &r.ReasoningEffort, &r.GoalIncarnationID, &r.Ending,
		&r.Steps, &r.DurationNs, &r.Usage, &r.ContextTokens, &r.UnresolvedEffects,
		&r.Capabilities,
		&r.MessageMark, &r.CreatedAt, &r.FinishedAt, &r.UpdatedAt, &r.InterruptsSuspended,
	); err != nil {
		return rundomain.Run{}, fmt.Errorf("scan run row: %w", err)
	}
	snapshot, err := r.snapshot()
	if err != nil {
		return rundomain.Run{}, err
	}
	if _, parked := snapshot.Lifecycle.(rundomain.Parked); parked {
		if err := r.validatePendingSet(pendingPolicy); err != nil {
			return rundomain.Run{}, err
		}
	}
	value, err := rundomain.Restore(snapshot)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", r.ID, err)
	}
	return value, nil
}

type runRow struct {
	ID                identity.RunID
	SessionID         identity.SessionID
	SpawnedByItemID   identity.ItemID
	ParentRunID       identity.RunID
	RootRunID         identity.RunID
	ActiveSegmentID   identity.SegmentID
	Outcome           string
	Provider          string
	Model             string
	ReasoningEffort   string
	GoalIncarnationID string
	Ending            string
	Steps             int
	Usage             string
	ContextTokens     int64
	UnresolvedEffects string

	MessageMark         sql.NullInt64
	Capabilities        string
	DurationNs          int64
	CreatedAt           int64
	FinishedAt          int64
	UpdatedAt           int64
	InterruptsSuspended sql.NullString
}

func encodeTerminalRun(value rundomain.Run) (runRow, error) {
	ending, finished := value.Ending()
	if !finished {
		return runRow{}, fmt.Errorf("sqlite: restore run %q: state is %s, want terminal", value.ID(), value.State())
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return runRow{}, fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	effects, err := encodeUnresolvedEffects(value.UnresolvedEffects())
	if err != nil {
		return runRow{}, err
	}
	outcome, encodedEnding, err := encodeRunEnding(ending)
	if err != nil {
		return runRow{}, err
	}
	capabilities, err := encodeRunCapabilities(value.Capabilities())
	if err != nil {
		return runRow{}, err
	}
	lineage, selection := value.Lineage(), value.ModelSelection()
	return runRow{
		ID: value.ID(), SessionID: value.SessionID(), SpawnedByItemID: lineage.SpawnedByItemID(),
		ParentRunID: lineage.ParentRunID(), RootRunID: lineage.RootRunID(), Outcome: outcome,
		Provider: selection.Provider(), Model: selection.Model(), ReasoningEffort: selection.ReasoningEffort(),
		GoalIncarnationID: value.GoalIncarnationID().String(), Ending: encodedEnding,
		Steps: metrics.steps, DurationNs: metrics.durationNs, Usage: metrics.usage,
		ContextTokens: value.ContextTokens(), UnresolvedEffects: effects, Capabilities: capabilities,
		MessageMark: messageMarkValue(value.MessageMark()),
		CreatedAt:   value.CreatedAt().UnixNano(), FinishedAt: value.FinishedAt().UnixNano(), UpdatedAt: value.UpdatedAt().UnixNano(),
	}, nil
}

func (r runRow) snapshot() (rundomain.Snapshot, error) {
	lineage, err := rundomain.NewLineage(r.SpawnedByItemID, r.ParentRunID, r.RootRunID)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("run %q: %w", r.ID, err)
	}
	capabilities, err := decodeRunCapabilities(r.Capabilities)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("run %q: %w", r.ID, err)
	}
	selection, err := flamemodel.NewWithReasoningEffort(r.Provider, r.Model, r.ReasoningEffort)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run %q model selection: %w", r.ID, err)
	}
	metrics, err := decodeRunMetrics(r.Usage, r.Steps, time.Duration(r.DurationNs))
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run %q metrics: %w", r.ID, err)
	}
	goalIncarnationID, _, err := goalref.ParseOptionalIncarnation(r.GoalIncarnationID)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run %q goal incarnation: %w", r.ID, err)
	}
	lifecycle, err := r.lifecycle()
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run %q lifecycle: %w", r.ID, err)
	}
	return rundomain.Snapshot{
		SessionID: r.SessionID, ID: r.ID, Lineage: lineage, ModelSelection: selection,
		GoalIncarnationID: goalIncarnationID, Lifecycle: lifecycle,
		Metrics: metrics, ContextTokens: r.ContextTokens, Capabilities: capabilities,
		CreatedAt: time.Unix(0, r.CreatedAt).UTC(), UpdatedAt: time.Unix(0, r.UpdatedAt).UTC(),
	}, nil
}

func (r runRow) lifecycle() (rundomain.Lifecycle, error) {
	effects, err := decodeUnresolvedEffects(r.UnresolvedEffects)
	if err != nil {
		return nil, fmt.Errorf("decode effects: %w", err)
	}
	if r.Outcome != "" {
		if !r.ActiveSegmentID.IsZero() {
			return nil, errors.New("a terminal Run stores an active Segment")
		}
		ending, err := decodeRunEnding(rundomain.Outcome(r.Outcome), r.Ending)
		if err != nil {
			return nil, fmt.Errorf("decode ending: %w", err)
		}
		return rundomain.Termination{
			Ending: ending, UnresolvedEffects: effects,
			FinishedAt: time.Unix(0, r.FinishedAt).UTC(), MessageMark: decodeMessageMark(r.MessageMark),
		}, nil
	}
	if r.Ending != "" || len(effects) != 0 || r.FinishedAt != 0 || r.MessageMark.Valid {
		return nil, errors.New("an open Run stores terminal facts")
	}
	if !r.ActiveSegmentID.IsZero() {
		return rundomain.ActiveSegment{ID: r.ActiveSegmentID}, nil
	}
	return rundomain.Parked{}, nil
}

func (r runRow) validatePendingSet(policy pendingReadPolicy) error {
	if !r.InterruptsSuspended.Valid {
		if policy == requirePendingSet {
			return fmt.Errorf("run %q is waiting with no root-owned Pending set", r.ID)
		}
		return nil
	}
	if _, err := decodeInterrupts(r.InterruptsSuspended.String); err != nil {
		return fmt.Errorf("decode run %q interrupts: %w", r.ID, err)
	}
	return nil
}

func decodeRunMetrics(encoded string, steps int, duration time.Duration) (rundomain.Metrics, error) {
	if encoded == "" {
		return rundomain.NewMetrics(nil, steps, duration)
	}
	var row *runAccountingRow
	if err := decodeStoredJSON([]byte(encoded), &row); err != nil {
		return rundomain.Metrics{}, err
	}
	if row == nil || (row.Reported == nil) == (len(row.UnreportedModels) == 0) {
		return rundomain.Metrics{}, errors.New("sqlite: expected one Run accounting variant")
	}
	if row.Reported == nil {
		return rundomain.RestoreUnreportedMetrics(row.UnreportedModels, steps, duration)
	}
	usage, err := row.Reported.usage()
	if err != nil {
		return rundomain.Metrics{}, err
	}
	return rundomain.NewMetrics(usage, steps, duration)
}

func (r runUsageRow) usage() (*accounting.Usage, error) {
	cost, err := accounting.CostFromOptionalNanoUSD(r.CostNanoUSD)
	if err != nil {
		return nil, err
	}
	total := accounting.Totals{
		Tokens: accounting.Tokens{
			InputTokens:      r.InputTokens,
			OutputTokens:     r.OutputTokens,
			CacheReadTokens:  r.CacheReadTokens,
			CacheWriteTokens: r.CacheWriteTokens,
			ReasoningTokens:  r.ReasoningTokens,
		},
		Cost: cost,
	}
	var byModel map[string]accounting.Totals
	if len(r.ByModel) > 0 {
		byModel = make(map[string]accounting.Totals, len(r.ByModel))
		for model, perModel := range r.ByModel {
			cost, err := accounting.CostFromOptionalNanoUSD(perModel.CostNanoUSD)
			if err != nil {
				return nil, fmt.Errorf("model %q: %w", model, err)
			}
			byModel[model] = accounting.Totals{
				Tokens: accounting.Tokens{
					InputTokens:      perModel.InputTokens,
					OutputTokens:     perModel.OutputTokens,
					CacheReadTokens:  perModel.CacheReadTokens,
					CacheWriteTokens: perModel.CacheWriteTokens,
					ReasoningTokens:  perModel.ReasoningTokens,
				},
				Cost: cost,
			}
		}
	}
	usage, err := accounting.NewUsage(total, byModel)
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

func (row runProblemRow) failure() (rundomain.Failure, error) {
	if !row.Kind.Valid() {
		return rundomain.Failure{}, fmt.Errorf("unknown run failure kind %q", row.Kind)
	}
	retryAfter, err := rundomain.RetryAfterFromSeconds(row.RetryAfterSeconds)
	if err != nil {
		return rundomain.Failure{}, fmt.Errorf("decode run failure: %w", err)
	}
	return rundomain.Failure{Kind: row.Kind, Detail: row.Detail, DocURL: row.DocURL, RetryAfter: retryAfter}, nil
}

func messageMarkValue(mark rundomain.MessageMark) sql.NullInt64 {
	count, known := mark.Count()
	return sql.NullInt64{Int64: int64(count), Valid: known}
}

func decodeMessageMark(value sql.NullInt64) rundomain.MessageMark {
	if !value.Valid {
		return rundomain.UnknownMessageMark()
	}
	return rundomain.MessageMarkAt(int(value.Int64))
}
