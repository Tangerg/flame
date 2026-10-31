package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// The Run row's JSON columns. Token accounting, a failure classification, and
// frozen capabilities are read and written whole with the row, never queried
// across, so they remain focused values instead of expanding into incidental
// columns. Their explicit adapter rows keep Go field names from defining the
// durable format.
type runUsageRow struct {
	InputTokens      int64                     `json:"inputTokens,omitzero"`
	OutputTokens     int64                     `json:"outputTokens,omitzero"`
	CacheReadTokens  int64                     `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64                     `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64                     `json:"reasoningTokens,omitzero"`
	CostUSD          *float64                  `json:"costUsd,omitzero"`
	ByModel          map[string]runModelRowUse `json:"byModel,omitempty"`
}

type runModelRowUse struct {
	InputTokens      int64    `json:"inputTokens,omitzero"`
	OutputTokens     int64    `json:"outputTokens,omitzero"`
	CacheReadTokens  int64    `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64    `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64    `json:"reasoningTokens,omitzero"`
	CostUSD          *float64 `json:"costUsd,omitzero"`
}

// runCapabilitiesRow is the Run's frozen optional behavior. Interrupt kinds are
// stored under their canonical names rather than ordinals, so inserting a kind
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
	encoded, err := encodeRunUsage(usage)
	if err != nil {
		return metricsValues{}, err
	}
	return metricsValues{
		steps:      metrics.Steps(),
		durationNs: int64(metrics.ActiveDuration()),
		usage:      encoded,
	}, nil
}

func encodeRunUsage(usage *accounting.Usage) (string, error) {
	row := runUsageRowOf(usage)
	if row == nil {
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
	row := &runUsageRow{
		InputTokens:      usage.Total.InputTokens,
		OutputTokens:     usage.Total.OutputTokens,
		CacheReadTokens:  usage.Total.CacheReadTokens,
		CacheWriteTokens: usage.Total.CacheWriteTokens,
		ReasoningTokens:  usage.Total.ReasoningTokens,
		CostUSD:          usage.Total.Cost.OptionalUSD(),
	}
	if len(usage.ByModel) > 0 {
		row.ByModel = make(map[string]runModelRowUse, len(usage.ByModel))
		for model, perModel := range usage.ByModel {
			row.ByModel[model] = runModelRowUse{
				InputTokens:      perModel.InputTokens,
				OutputTokens:     perModel.OutputTokens,
				CacheReadTokens:  perModel.CacheReadTokens,
				CacheWriteTokens: perModel.CacheWriteTokens,
				ReasoningTokens:  perModel.ReasoningTokens,
				CostUSD:          perModel.Cost.OptionalUSD(),
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
	var (
		id                string
		sessionID         string
		spawnedByItemID   string
		parentRunID       string
		rootRunID         string
		coarse            string
		activeSegmentID   string
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
	)
	if err := row.Scan(
		&id, &sessionID,
		&spawnedByItemID, &parentRunID, &rootRunID,
		&coarse, &activeSegmentID, &outcome,
		&provider, &model, &reasoningEffort, &goalIncarnationID, &ending,
		&steps, &durationNs, &usage, &contextTokens, &unresolvedEffects,
		&capabilities,
		&messageMark, &createdAt, &finishedAt, &updatedAt, &interruptsSuspended,
	); err != nil {
		return rundomain.Run{}, fmt.Errorf("scan run row: %w", err)
	}
	storedState, err := parseRunState(coarse)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", id, err)
	}
	lineage, err := rundomain.NewLineage(spawnedByItemID, parentRunID, rootRunID)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", id, err)
	}
	capabilitiesValue, err := decodeRunCapabilities(capabilities)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", id, err)
	}
	selection, err := modelchoice.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run %q model selection: %w", id, err)
	}
	decodedUsage, err := decodeRunUsage(usage)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run %q usage: %w", id, err)
	}
	metrics, err := rundomain.NewMetrics(decodedUsage, steps, time.Duration(durationNs))
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run %q metrics: %w", id, err)
	}
	snapshot := rundomain.Snapshot{
		SessionID: sessionID, ID: id, Lineage: lineage, ModelSelection: selection,
		GoalIncarnationID: goalIncarnationID, ActiveSegmentID: activeSegmentID,
		Metrics: metrics, ContextTokens: contextTokens,
		Capabilities: capabilitiesValue,
		CreatedAt:    time.Unix(0, createdAt).UTC(), UpdatedAt: time.Unix(0, updatedAt).UTC(),
		MessageMark: decodeMessageMark(messageMark),
	}

	snapshot.UnresolvedEffects, err = decodeUnresolvedEffects(unresolvedEffects)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run effects: %w", err)
	}
	if snapshot.Ending, err = decodeRunEnding(outcome, ending); err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run %q ending: %w", id, err)
	}
	// Zero encodes absence on open rows and the Unix epoch on terminal rows.
	// Retain nonzero finish times on open rows so the Domain rejects them.
	if finishedAt != 0 || storedState == rundomain.Finished {
		snapshot.FinishedAt = time.Unix(0, finishedAt).UTC()
	}
	switch storedState {
	case rundomain.Running:
		snapshot.State = rundomain.Running
	case rundomain.Waiting:
		snapshot.State = rundomain.Waiting
		// Every suspended Run must join its root-owned pending set. A Run carries
		// no interrupts of its own, so the joined column is read only to prove the
		// set is intact: a waiting Run whose pending payload cannot be decoded is
		// unusable, and saying so here names the Run instead of failing later
		// wherever that set is finally read.
		if !interruptsSuspended.Valid {
			if pendingPolicy == requirePendingSet {
				return rundomain.Run{}, fmt.Errorf("run %q is waiting with no root-owned Pending set", id)
			}
			break
		}
		if _, decodeErr := decodeInterrupts(interruptsSuspended.String); decodeErr != nil {
			return rundomain.Run{}, fmt.Errorf("decode run %q interrupts: %w", id, decodeErr)
		}
	case rundomain.Finished:
		snapshot.State = rundomain.Finished
	}
	value, err := rundomain.Restore(snapshot)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", id, err)
	}
	return value, nil
}

func decodeRunUsage(encoded string) (*accounting.Usage, error) {
	if encoded == "" {
		return nil, nil
	}
	var row *runUsageRow
	if err := decodeStoredJSON([]byte(encoded), &row); err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("decode run usage: stored usage must be an object")
	}
	return row.usage()
}

func (r runUsageRow) usage() (*accounting.Usage, error) {
	cost, err := accounting.CostFromOptional(r.CostUSD)
	if err != nil {
		return nil, err
	}
	usage := &accounting.Usage{Total: accounting.Totals{
		Tokens: accounting.Tokens{
			InputTokens:      r.InputTokens,
			OutputTokens:     r.OutputTokens,
			CacheReadTokens:  r.CacheReadTokens,
			CacheWriteTokens: r.CacheWriteTokens,
			ReasoningTokens:  r.ReasoningTokens,
		},
		Cost: cost,
	}}
	if len(r.ByModel) > 0 {
		usage.ByModel = make(map[string]accounting.Totals, len(r.ByModel))
		for model, perModel := range r.ByModel {
			cost, err := accounting.CostFromOptional(perModel.CostUSD)
			if err != nil {
				return nil, fmt.Errorf("model %q: %w", model, err)
			}
			usage.ByModel[model] = accounting.Totals{
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
	return usage, nil
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
