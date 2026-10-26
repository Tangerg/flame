package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	runtimeidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
	sqlitebase "github.com/Tangerg/flame/packages/go-sdk/storage/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type interruptState string

const interruptStateOpen interruptState = "open"

func (i interruptState) databaseValue() string { return string(i) }

// InterruptStore is the SQLite-backed registry of root-owned pending interrupt
// sets. The typed domain values are encoded through explicit adapter rows;
// protocol payloads and Go field names never define this storage shape.
type InterruptStore struct {
	db *sql.DB
}

// pendingRecord is SQLite's technical representation of one durable
// waiting-tree hand-off. Application semantics belong to the persistence
// adapter; this record only names the values required by the storage codec.
type pendingRecord struct {
	RootRunID     string
	ExecutorID    string
	Interrupts    []openInterruptRecord
	Bindings      []interruptBindingRecord
	Continuations []continuationRecord
	CreatedAt     time.Time
}

// continuationRecord is the stored continuation row for one Run.
type continuationRecord struct {
	RunID        string
	MemberID     string
	DrainedTools []drainedToolRecord
}

// interruptBindingRecord is the stored item-to-input-request correspondence.
type interruptBindingRecord struct {
	InterruptItemID   string
	MemberID          string
	RequestID         string
	ToolCallID        string
	ToolRef           string
	SourceFingerprint string
}

// openInterruptRecord is the stored open interrupt: the Item it names and, for
// an approval, the policy's review. The Item owns everything else.
type openInterruptRecord struct {
	ItemID   string
	Approval *approvalReviewRecord
}

// approvalReviewRecord is the stored approval policy review of one held call.
type approvalReviewRecord struct {
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

// drainedToolRecord is the stored identity of an open tool invocation.
type drainedToolRecord struct {
	ItemID       string
	CallID       string
	SourceCallID string
}

func (i pendingRecord) rootContinuation() (continuationRecord, bool) {
	for _, continuation := range i.Continuations {
		if continuation.RunID == i.RootRunID {
			return continuation, true
		}
	}
	return continuationRecord{}, false
}

func (i pendingRecord) validateStorageShape() error {
	if err := instant.Validate(i.CreatedAt); err != nil {
		return fmt.Errorf("creation time: %w", err)
	}
	if err := resourceid.ValidateRun(i.RootRunID); err != nil {
		return err
	}
	if err := runtimeidentity.ValidateExecutor(i.ExecutorID); err != nil {
		return err
	}
	switch {
	case i.CreatedAt.IsZero():
		return errors.New("creation time is required")
	case len(i.Interrupts) == 0:
		return errors.New("interrupt payload is required")
	case len(i.Continuations) == 0:
		return errors.New("continuation payload is required")
	case len(i.Bindings) != len(i.Interrupts):
		return errors.New("interrupt bindings do not match interrupts")
	}
	root, ok := i.rootContinuation()
	if !ok {
		return errors.New("root continuation and member ID are required")
	}
	if err := runtimeidentity.ValidateMember(root.MemberID); err != nil {
		return err
	}
	return nil
}

type drainedToolRow struct {
	ItemID       string `json:"itemId"`
	CallID       string `json:"callId"`
	SourceCallID string `json:"sourceCallId,omitempty"`
}

type interruptPayload struct {
	ItemID   string           `json:"itemId"`
	Approval *approvalPayload `json:"approval,omitzero"`
}

type approvalPayload struct {
	Risk         string `json:"risk"`
	Reason       string `json:"reason,omitempty"`
	Rememberable bool   `json:"rememberable,omitzero"`
}

type continuationRow struct {
	RunID        string           `json:"runId"`
	MemberID     string           `json:"memberId"`
	DrainedTools []drainedToolRow `json:"drainedTools,omitempty"`
}

type interruptBindingRow struct {
	InterruptItemID   string `json:"interruptItemId"`
	MemberID          string `json:"memberId"`
	RequestID         string `json:"requestId"`
	ToolCallID        string `json:"toolCallId,omitempty"`
	ToolRef           string `json:"toolRef,omitempty"`
	SourceFingerprint string `json:"sourceFingerprint,omitempty"`
}

// NewInterruptStore binds the SQLite interrupt registry to a database opened via
// [openDatabase].
func NewInterruptStore(db *sql.DB) *InterruptStore {
	return &InterruptStore{db: db}
}

// Open records a newly reached barrier. An existing root Run or executor root
// is an identity conflict; a barrier is replaced only after its owner consumes
// the previous one (Consume) in the same application transaction, so this insert
// never updates a surviving row.
func (s *InterruptStore) insertPending(ctx context.Context, p pendingRecord) error {
	if err := p.validateStorageShape(); err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	root, _ := p.rootContinuation()
	payload, err := encodeStoredJSON(interruptPayloads(p.Interrupts))
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupts: %w", err)
	}
	continuationValues := continuationRows(p.Continuations)
	continuations, err := encodeStoredJSON(continuationValues)
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt continuations: %w", err)
	}
	bindings, err := encodeStoredJSON(interruptBindingRows(p.Bindings))
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt bindings: %w", err)
	}
	if err := s.requireTreeRuns(ctx, p); err != nil {
		return err
	}
	result, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx,
		`INSERT INTO interrupts(root_run_id, executor_id, root_member_id, payload, continuations, interrupt_bindings, created_at, state)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.RootRunID,
		p.ExecutorID,
		root.MemberID,
		string(payload),
		string(continuations),
		string(bindings),
		p.CreatedAt.UnixNano(),
		interruptStateOpen.databaseValue(),
	)
	if isUniqueViolation(err) {
		return fmt.Errorf(
			"%w: Pending root Run %q or executor root %q is already claimed",
			transcript.ErrIdentityConflict,
			p.RootRunID,
			root.MemberID,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect opened interrupt: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("%w: Pending root Run %q is already open", transcript.ErrIdentityConflict, p.RootRunID)
	}
	return nil
}

// interruptColumns reads one hand-off. Its Runs own their lineage,
// capabilities and Goal incarnation; the hand-off only names them.
const interruptColumns = `root_run_id, executor_id,
	root_member_id, payload, continuations, interrupt_bindings, created_at`

// requireTreeRuns proves, in the hand-off's transaction, that the Runs it names
// are a root Run and that root's descendants.
func (s *InterruptStore) requireTreeRuns(ctx context.Context, p pendingRecord) error {
	var members sql.NullString
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT (SELECT json_group_array(member.run_id)
		           FROM runs AS member
		          WHERE member.session_id = root.session_id
		            AND (member.run_id = root.run_id OR member.root_run_id = root.run_id))
		   FROM runs AS root WHERE root.run_id = ? AND root.root_run_id = ''`,
		p.RootRunID,
	).Scan(&members)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: Pending root Run %q is not a root Run", transcript.ErrIdentityConflict, p.RootRunID)
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect Pending root Run %q: %w", p.RootRunID, err)
	}
	var runIDs []string
	if err := decodeStoredJSON([]byte(members.String), &runIDs); err != nil {
		return fmt.Errorf("sqlite: decode Pending tree Runs: %w", err)
	}
	for _, continuation := range p.Continuations {
		if !slices.Contains(runIDs, continuation.RunID) {
			return fmt.Errorf("%w: Pending continuation %q is not a Run of its tree", transcript.ErrIdentityConflict, continuation.RunID)
		}
	}
	return nil
}

func (s *InterruptStore) pendingRecords(ctx context.Context, sessionID string) ([]pendingRecord, error) {
	return s.list(ctx, sessionID, "", 0, "", 0)
}

// ListPage returns open interrupts oldest first, bounded by the query. after is
// the (open time, run id) position a previous page ended at; the pair is what
// makes the order total, since two runs can park in the same nanosecond.
func (s *InterruptStore) pendingRecordsPage(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRootRunID string, limit int) ([]pendingRecord, error) {
	return s.list(ctx, sessionID, rootRunID, afterCreatedAt, afterRootRunID, limit)
}

func (s *InterruptStore) list(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRunID string, limit int) ([]pendingRecord, error) {
	if err := validateOptionalSessionResource("list interrupts", sessionID); err != nil {
		return nil, err
	}
	if err := validateOptionalRunResource("list interrupts root", rootRunID); err != nil {
		return nil, err
	}
	if err := validateOptionalRunResource("list interrupts anchor", afterRunID); err != nil {
		return nil, err
	}
	query := `SELECT ` + interruptColumns + ` FROM interrupts`
	args := []any{interruptStateOpen.databaseValue()}
	conditions := []string{`state = ?`}
	if sessionID != "" {
		conditions = append(conditions, `root_run_id IN (SELECT run_id FROM runs WHERE session_id = ?)`)
		args = append(args, sessionID)
	}
	if rootRunID != "" {
		conditions = append(conditions, `root_run_id = ?`)
		args = append(args, rootRunID)
	}
	if afterCreatedAt > 0 || afterRunID != "" {
		conditions = append(conditions, `(created_at > ? OR (created_at = ? AND root_run_id > ?))`)
		args = append(args, afterCreatedAt, afterCreatedAt, afterRunID)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY created_at, root_run_id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list interrupts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]pendingRecord, 0)
	for rows.Next() {
		p, err := scanPending(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list interrupts: %w", err)
	}
	return out, nil
}

func (s *InterruptStore) loadPending(ctx context.Context, runID string) (pendingRecord, bool, error) {
	if err := validateRunResource("read interrupt", runID); err != nil {
		return pendingRecord{}, false, err
	}
	row := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+interruptColumns+` FROM interrupts WHERE root_run_id = ? AND state = ?`,
		runID, interruptStateOpen.databaseValue())
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return pendingRecord{}, false, nil
	}
	if err != nil {
		return pendingRecord{}, false, err
	}
	return p, true, nil
}

// Consume atomically reads AND deletes the pending interrupt for runID (one
// DELETE ... RETURNING), or returns ok=false when none is recorded — the resume
// claim contract. A single statement means two concurrent resumes can't both
// observe the same open interrupt: one claims it, the other gets ok=false, so a
// non-idempotent tool never re-fires.
func (s *InterruptStore) consumePending(ctx context.Context, runID string) (pendingRecord, bool, error) {
	if err := resourceid.ValidateRun(runID); err != nil {
		return pendingRecord{}, false, fmt.Errorf("sqlite: consume interrupt: %w", err)
	}
	row := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx,
		`DELETE FROM interrupts WHERE root_run_id = ? AND state = ?
		 RETURNING `+interruptColumns,
		runID, interruptStateOpen.databaseValue())
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return pendingRecord{}, false, nil
	}
	if err != nil {
		return pendingRecord{}, false, err
	}
	return p, true, nil
}

func (s *InterruptStore) Delete(ctx context.Context, runID string) error {
	if err := resourceid.ValidateRun(runID); err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	if _, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx,
		`DELETE FROM interrupts WHERE root_run_id = ?`, runID,
	); err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	return nil
}

// scanRow abstracts *sql.Row and *sql.Rows so one scan path serves Get +
// List.
func scanPending(row scanRow) (pendingRecord, error) {
	var (
		p               pendingRecord
		payload         string
		rootMemberID    string
		continuations   string
		encodedBindings string
		createdNs       int64
	)
	if err := row.Scan(
		&p.RootRunID,
		&p.ExecutorID,
		&rootMemberID,
		&payload,
		&continuations,
		&encodedBindings,
		&createdNs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return pendingRecord{}, err
		}
		return pendingRecord{}, fmt.Errorf("sqlite: scan interrupt: %w", err)
	}
	var err error
	if p.Interrupts, err = decodeInterrupts(payload); err != nil {
		return pendingRecord{}, fmt.Errorf("sqlite: decode interrupts: %w", err)
	}
	var continuationValues []continuationRow
	if decodeInterruptJSONErr := decodeStoredJSON([]byte(continuations), &continuationValues); decodeInterruptJSONErr != nil {
		return pendingRecord{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", decodeInterruptJSONErr)
	}
	p.Continuations = continuationsFromRows(continuationValues)
	var bindingValues []interruptBindingRow
	if decodeInterruptJSONErr := decodeStoredJSON([]byte(encodedBindings), &bindingValues); decodeInterruptJSONErr != nil {
		return pendingRecord{}, fmt.Errorf("sqlite: decode input-request bindings: %w", decodeInterruptJSONErr)
	}
	p.Bindings = interruptBindingsFromRows(bindingValues)
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	if err := p.validateStorageShape(); err != nil {
		return pendingRecord{}, fmt.Errorf("sqlite: decode interrupt %q: %w", p.RootRunID, err)
	}
	root, _ := p.rootContinuation()
	if root.MemberID != rootMemberID {
		return pendingRecord{}, fmt.Errorf(
			"sqlite: decode interrupt %q: root member index %q does not match continuation %q",
			p.RootRunID,
			rootMemberID,
			root.MemberID,
		)
	}
	return p, nil
}

// decodeInterrupts reads the stored open-interrupt set. It is the one reader of
// that encoding: the Run read joins the same column to prove a parked Run's set
// is intact, and a second decoder there could disagree about the format.
func decodeInterrupts(payload string) ([]openInterruptRecord, error) {
	if payload == "" {
		return nil, nil
	}
	var rows []interruptPayload
	if err := decodeStoredJSON([]byte(payload), &rows); err != nil {
		return nil, err
	}
	return interruptsFromPayloads(rows)
}

func drainedToolRows(tools []drainedToolRecord) []drainedToolRow {
	rows := make([]drainedToolRow, len(tools))
	for index, tool := range tools {
		rows[index] = drainedToolRow(tool)
	}
	return rows
}

func drainedToolsFromRows(rows []drainedToolRow) []drainedToolRecord {
	tools := make([]drainedToolRecord, len(rows))
	for index, row := range rows {
		tools[index] = drainedToolRecord(row)
	}
	return tools
}

func continuationRows(values []continuationRecord) []continuationRow {
	rows := make([]continuationRow, len(values))
	for index, value := range values {
		rows[index] = continuationRow{
			RunID:        value.RunID,
			MemberID:     value.MemberID,
			DrainedTools: drainedToolRows(value.DrainedTools),
		}
	}
	return rows
}

func continuationsFromRows(rows []continuationRow) []continuationRecord {
	values := make([]continuationRecord, len(rows))
	for index, row := range rows {
		values[index] = continuationRecord{
			RunID:        row.RunID,
			MemberID:     row.MemberID,
			DrainedTools: drainedToolsFromRows(row.DrainedTools),
		}
	}
	return values
}

func interruptPayloads(values []openInterruptRecord) []interruptPayload {
	rows := make([]interruptPayload, len(values))
	for index, value := range values {
		rows[index] = interruptPayload{ItemID: value.ItemID}
		if review := value.Approval; review != nil {
			rows[index].Approval = &approvalPayload{
				Risk: string(review.Risk), Reason: review.Reason, Rememberable: review.Rememberable,
			}
		}
	}
	return rows
}

func interruptsFromPayloads(rows []interruptPayload) ([]openInterruptRecord, error) {
	values := make([]openInterruptRecord, len(rows))
	for index, row := range rows {
		values[index] = openInterruptRecord{ItemID: row.ItemID}
		if review := row.Approval; review != nil {
			risk := tool.RiskLevel(review.Risk)
			if !risk.Valid() {
				return nil, fmt.Errorf("interrupt[%d] approval has unknown risk %q", index, review.Risk)
			}
			values[index].Approval = &approvalReviewRecord{
				Risk: risk, Reason: review.Reason, Rememberable: review.Rememberable,
			}
		}
	}
	return values, nil
}

func interruptBindingRows(values []interruptBindingRecord) []interruptBindingRow {
	rows := make([]interruptBindingRow, len(values))
	for index, value := range values {
		rows[index] = interruptBindingRow(value)
	}
	return rows
}

func interruptBindingsFromRows(rows []interruptBindingRow) []interruptBindingRecord {
	values := make([]interruptBindingRecord, len(rows))
	for index, row := range rows {
		values[index] = interruptBindingRecord(row)
	}
	return values
}
