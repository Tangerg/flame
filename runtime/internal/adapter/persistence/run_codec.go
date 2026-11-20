package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// The Run row's JSON columns. Token accounting, a failure classification, and
// frozen capabilities are read and written whole with the row, never queried
// across, so they remain focused values instead of expanding into incidental
// columns. Their explicit adapter rows keep Go field names from defining the
// durable format.
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

// runCapabilitiesRow is the Run's frozen optional behavior. Interrupt types are
// stored under their canonical names rather than ordinals, so inserting a type
// into the enum cannot silently re-label stored rows.
type runCapabilitiesRow struct {
	ChildRuns      bool                      `json:"childRuns,omitzero"`
	InterruptTypes []rundomain.InterruptType `json:"interruptTypes,omitempty"`
}

// encodeRunCapabilities returns the empty string for no optional capabilities,
// keeping one representation instead of both null and an empty object.
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

// metricsValues are one Run's accumulated consumption, encoded and ready to bind
// to a statement. An empty usage string means the Run has recorded none yet.
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

// runEndingRow is the stored ending. The outcome column names the variant;
// this record holds only that variant's facts.
type runEndingRow struct {
	Detail  string         `json:"detail,omitempty"`
	Failure *runProblemRow `json:"failure,omitzero"`
	Loss    *runLossRow    `json:"loss,omitzero"`
}

type runLossRow struct {
	Reason rundomain.LossReason `json:"reason"`
	Detail string               `json:"detail,omitempty"`
}

func encodeRunEnding(ending rundomain.Ending) (outcome, encoded string, err error) {
	var row runEndingRow
	switch ending := ending.(type) {
	case nil:
		return "", "", nil
	case rundomain.Completed:
		return string(rundomain.OutcomeCompleted), "", nil
	case rundomain.Canceled:
		row.Detail = ending.Detail
	case rundomain.TimedOut:
		row.Detail = ending.Detail
	case rundomain.Failed:
		row.Failure = &runProblemRow{
			Kind: ending.Failure.Kind, Detail: ending.Failure.Detail, DocURL: ending.Failure.DocURL,
			RetryAfterSeconds: ending.Failure.RetryAfterSeconds(),
		}
	case rundomain.Lost:
		row.Loss = &runLossRow{Reason: ending.Loss.Reason, Detail: ending.Loss.Detail}
	default:
		return "", "", fmt.Errorf("encode run ending: unknown ending %T", ending)
	}
	data, err := encodeStoredJSON(row)
	if err != nil {
		return "", "", fmt.Errorf("encode run ending: %w", err)
	}
	return string(ending.Outcome()), string(data), nil
}

func decodeRunEnding(outcome, encoded string) (rundomain.Ending, error) {
	if outcome == "" {
		if encoded != "" {
			return nil, errors.New("an open Run stores ending facts")
		}
		return nil, nil
	}
	reason, ok := rundomain.ParseOutcome(outcome)
	if !ok {
		return nil, fmt.Errorf("unknown outcome %q", outcome)
	}
	var row runEndingRow
	if reason != rundomain.OutcomeCompleted {
		if err := decodeStoredJSON([]byte(encoded), &row); err != nil {
			return nil, err
		}
	}
	switch reason {
	case rundomain.OutcomeCompleted:
		if encoded != "" {
			return nil, errors.New("a completed Run stores ending facts")
		}
		return rundomain.Completed{}, nil
	case rundomain.OutcomeCanceled:
		return rundomain.Canceled{Detail: row.Detail}, nil
	case rundomain.OutcomeTimedOut:
		return rundomain.TimedOut{Detail: row.Detail}, nil
	case rundomain.OutcomeFailed:
		if row.Failure == nil {
			return nil, errors.New("a failed Run stores no failure")
		}
		failure, err := row.Failure.failure()
		if err != nil {
			return nil, err
		}
		return rundomain.Failed{Failure: failure}, nil
	case rundomain.OutcomeLost:
		if row.Loss == nil {
			return nil, errors.New("a lost Run stores no loss")
		}
		return rundomain.Lost{Loss: rundomain.Loss{Reason: row.Loss.Reason, Detail: row.Loss.Detail}}, nil
	default:
		return nil, fmt.Errorf("unknown outcome %q", outcome)
	}
}

// pendingReadPolicy says whether an Waiting Run must have its root-owned
// Pending set. Ordinary reads require it because a complete parked
// Run is inseparable from that set. Boot recovery is the one exception: it must
// still be able to read and terminalize a row whose pending set was lost in the
// crash it is repairing.
type pendingReadPolicy uint8

const (
	requirePendingSet pendingReadPolicy = iota
	allowMissingPendingSet
)

// scanRun decodes one complete Run row plus the joined open-interrupt payload.
//
// The fine [rundomain.State] is rebuilt from the coarse admission state and
// the terminal reason beside it rather than stored a second time, and the
// terminal facts are materialized exactly when the state says they exist — the
// equivalence [rundomain.Run.Validate] enforces on the way in.
func scanRun(row scanRow) (rundomain.Run, error) {
	return scanRunRow(row, requirePendingSet)
}

// scanRunForRecovery uses the same complete durable Run decoder as every normal
// read, but tolerates the one broken relation reconciliation exists to repair:
// an Waiting row whose root-owned pending set is missing.
func scanRunForRecovery(row scanRow) (rundomain.Run, error) {
	return scanRunRow(row, allowMissingPendingSet)
}

func scanRunRow(row scanRow, pendingPolicy pendingReadPolicy) (rundomain.Run, error) {
	var r runRow
	if err := row.Scan(
		&r.id, &r.sessionID,
		&r.spawnedByItemID, &r.parentRunID, &r.rootRunID,
		&r.coarse, &r.activeSegmentID, &r.outcome,
		&r.provider, &r.model, &r.reasoningEffort, &r.goalIncarnationID, &r.ending,
		&r.steps, &r.durationNs, &r.usage, &r.contextTokens, &r.unresolvedEffects,
		&r.capabilities,
		&r.messageMark, &r.createdAt, &r.finishedAt, &r.updatedAt, &r.interruptsSuspended,
	); err != nil {
		return rundomain.Run{}, fmt.Errorf("scan run row: %w", err)
	}
	snapshot, err := r.snapshot()
	if err != nil {
		return rundomain.Run{}, err
	}
	if snapshot.State == rundomain.Waiting {
		if err := r.validatePendingSet(pendingPolicy); err != nil {
			return rundomain.Run{}, err
		}
	}
	value, err := rundomain.Restore(snapshot)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", r.id, err)
	}
	return value, nil
}

// runRow is one stored Run as its columns hold it.
type runRow struct {
	id                identity.RunID
	sessionID         identity.SessionID
	spawnedByItemID   identity.ItemID
	parentRunID       identity.RunID
	rootRunID         identity.RunID
	coarse            string
	activeSegmentID   identity.SegmentID
	outcome           string
	provider          string
	model             string
	reasoningEffort   string
	goalIncarnationID string
	ending            string
	steps             int
	usage             string
	contextTokens     int64
	unresolvedEffects string

	messageMark         sql.NullInt64
	capabilities        string
	durationNs          int64
	createdAt           int64
	finishedAt          int64
	updatedAt           int64
	interruptsSuspended sql.NullString
}

func (r runRow) snapshot() (rundomain.Snapshot, error) {
	state, err := parseRunState(r.coarse)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("run %q: %w", r.id, err)
	}
	lineage, err := rundomain.NewLineage(r.spawnedByItemID, r.parentRunID, r.rootRunID)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("run %q: %w", r.id, err)
	}
	capabilities, err := decodeRunCapabilities(r.capabilities)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("run %q: %w", r.id, err)
	}
	selection, err := modelchoice.NewWithReasoningEffort(r.provider, r.model, r.reasoningEffort)
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run %q model selection: %w", r.id, err)
	}
	metrics, err := decodeRunMetrics(r.usage, r.steps, time.Duration(r.durationNs))
	if err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run %q metrics: %w", r.id, err)
	}
	snapshot := rundomain.Snapshot{
		SessionID: r.sessionID, ID: r.id, State: state, Lineage: lineage, ModelSelection: selection,
		GoalIncarnationID: r.goalIncarnationID, ActiveSegmentID: r.activeSegmentID,
		Metrics: metrics, ContextTokens: r.contextTokens,
		Capabilities: capabilities,
		CreatedAt:    time.Unix(0, r.createdAt).UTC(), UpdatedAt: time.Unix(0, r.updatedAt).UTC(),
		MessageMark: decodeMessageMark(r.messageMark),
	}
	if snapshot.UnresolvedEffects, err = decodeUnresolvedEffects(r.unresolvedEffects); err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run effects: %w", err)
	}
	if snapshot.Ending, err = decodeRunEnding(r.outcome, r.ending); err != nil {
		return rundomain.Snapshot{}, fmt.Errorf("decode run %q ending: %w", r.id, err)
	}
	// Zero encodes absence on open rows and the Unix epoch on terminal rows.
	// Retain nonzero finish times on open rows so the Domain rejects them.
	if r.finishedAt != 0 || state == rundomain.Finished {
		snapshot.FinishedAt = time.Unix(0, r.finishedAt).UTC()
	}
	return snapshot, nil
}

// validatePendingSet proves a waiting Run joins an intact root-owned pending
// set. A Run carries no interrupts of its own, so the joined column is read
// only to prove the set decodes: saying so here names the Run instead of
// failing later wherever that set is finally read.
func (r runRow) validatePendingSet(policy pendingReadPolicy) error {
	if !r.interruptsSuspended.Valid {
		if policy == requirePendingSet {
			return fmt.Errorf("run %q is waiting with no root-owned Pending set", r.id)
		}
		return nil
	}
	if _, err := decodeInterrupts(r.interruptsSuspended.String); err != nil {
		return fmt.Errorf("decode run %q interrupts: %w", r.id, err)
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

// messageMarkValue stores an unknown watermark as NULL.
func messageMarkValue(mark rundomain.MessageMark) any {
	if count, known := mark.Count(); known {
		return count
	}
	return nil
}

func decodeMessageMark(value sql.NullInt64) rundomain.MessageMark {
	if !value.Valid {
		return rundomain.UnknownMessageMark()
	}
	return rundomain.MessageMarkAt(int(value.Int64))
}
