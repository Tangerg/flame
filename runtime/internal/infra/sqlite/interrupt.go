package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

type interruptState string

const (
	interruptStateOpen     interruptState = "open"
	interruptStateResuming interruptState = "resuming"
)

func (i interruptState) databaseValue() string { return string(i) }

// InterruptStore is the SQLite-backed registry of root-owned pending interrupt
// sets. The typed domain values are encoded through explicit adapter rows;
// protocol payloads and Go field names never define this storage shape.
type InterruptStore struct {
	db *sql.DB
}

// InterruptRecord is SQLite's technical representation of one durable
// waiting-tree hand-off. Application semantics belong to the persistence
// adapter; this record only names the values required by the storage codec.
type InterruptRecord struct {
	RootRunID     string
	SessionID     string
	ExecutorID    string
	Interrupts    []OpenInterruptRecord
	Bindings      []InterruptBindingRecord
	Continuations []ContinuationRecord
	CreatedAt     time.Time
}

// ContinuationRecord is the stored continuation row for one Run.
type ContinuationRecord struct {
	RunID        string
	MemberID     string
	DrainedTools []DrainedToolRecord
}

// InterruptBindingRecord is the stored item-to-input-request correspondence.
type InterruptBindingRecord struct {
	InterruptItemID string
	MemberID        string
	RequestID       string
	ToolCallID      string
}

// OpenInterruptRecord is the stored open interrupt: the Item it names and, for
// an approval, the policy's review. The Item owns everything else.
type OpenInterruptRecord struct {
	ItemID   string
	Approval *ApprovalReviewRecord
}

// ApprovalReviewRecord is the stored approval policy review of one held call.
type ApprovalReviewRecord struct {
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

// DrainedToolRecord is the stored identity of an open tool invocation.
type DrainedToolRecord struct {
	ItemID       string
	CallID       string
	SourceCallID string
}

func (i InterruptRecord) rootContinuation() (ContinuationRecord, bool) {
	for _, continuation := range i.Continuations {
		if continuation.RunID == i.RootRunID {
			return continuation, true
		}
	}
	return ContinuationRecord{}, false
}

func (i InterruptRecord) validateStorageShape() error {
	if err := resourceid.ValidateRun(i.RootRunID); err != nil {
		return err
	}
	if err := resourceid.ValidateSession(i.SessionID); err != nil {
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
	InterruptItemID string `json:"interruptItemId"`
	MemberID        string `json:"memberId"`
	RequestID       string `json:"requestId"`
	ToolCallID      string `json:"toolCallId,omitempty"`
}

// NewInterruptStore binds the SQLite interrupt registry to a database opened via
// [Open].
func NewInterruptStore(db *sql.DB) *InterruptStore {
	return &InterruptStore{db: db}
}

// Open records a newly reached barrier. An existing root Run or executor root
// is an identity conflict; a barrier is replaced only after its owner consumes
// the previous one in the same application transaction.
func (i *InterruptStore) Open(ctx context.Context, p InterruptRecord) error {
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
	if err := i.requireTreeRuns(ctx, p); err != nil {
		return err
	}
	result, err := conn(ctx, i.db).ExecContext(ctx,
		`INSERT INTO interrupts(root_run_id, session_id, executor_id, root_member_id, payload, continuations, interrupt_bindings, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(root_run_id) DO UPDATE SET
		   executor_id = excluded.executor_id,
		   root_member_id = excluded.root_member_id,
		   payload = excluded.payload,
		   continuations = excluded.continuations,
		   interrupt_bindings = excluded.interrupt_bindings,
		   created_at = excluded.created_at,
		   state = ?
		 WHERE interrupts.state = ?
		   AND interrupts.session_id = excluded.session_id
		   AND interrupts.executor_id = excluded.executor_id
		   AND interrupts.root_member_id = excluded.root_member_id`,
		p.RootRunID,
		p.SessionID,
		p.ExecutorID,
		root.MemberID,
		string(payload),
		string(continuations),
		string(bindings),
		p.CreatedAt.UnixNano(),
		interruptStateOpen.databaseValue(),
		interruptStateResuming.databaseValue(),
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
const interruptColumns = `root_run_id, session_id, executor_id,
	root_member_id, payload, continuations, interrupt_bindings, created_at`

// requireTreeRuns proves, in the hand-off's transaction, that the Runs it names
// are its root Run in this Session and that root's descendants.
func (i *InterruptStore) requireTreeRuns(ctx context.Context, p InterruptRecord) error {
	var members sql.NullString
	err := conn(ctx, i.db).QueryRowContext(ctx,
		`SELECT (SELECT json_group_array(member.run_id)
		           FROM runs AS member
		          WHERE member.session_id = root.session_id
		            AND (member.run_id = root.run_id OR member.root_run_id = root.run_id))
		   FROM runs AS root WHERE root.run_id = ? AND root.session_id = ? AND root.root_run_id = ''`,
		p.RootRunID, p.SessionID,
	).Scan(&members)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: Pending root Run %q is not a root Run of Session %q", transcript.ErrIdentityConflict, p.RootRunID, p.SessionID)
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

func (i *InterruptStore) List(ctx context.Context, sessionID string) ([]InterruptRecord, error) {
	return i.list(ctx, sessionID, "", 0, "", 0)
}

// ListPage returns open interrupts oldest first, bounded by the query. after is
// the (open time, run id) position a previous page ended at; the pair is what
// makes the order total, since two runs can park in the same nanosecond.
func (i *InterruptStore) ListPage(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRootRunID string, limit int) ([]InterruptRecord, error) {
	return i.list(ctx, sessionID, rootRunID, afterCreatedAt, afterRootRunID, limit)
}

func (i *InterruptStore) list(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRunID string, limit int) ([]InterruptRecord, error) {
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
		conditions = append(conditions, `session_id = ?`)
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

	rows, err := conn(ctx, i.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list interrupts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]InterruptRecord, 0)
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

func (i *InterruptStore) Get(ctx context.Context, runID string) (InterruptRecord, bool, error) {
	if err := validateRunResource("read interrupt", runID); err != nil {
		return InterruptRecord{}, false, err
	}
	row := conn(ctx, i.db).QueryRowContext(ctx,
		`SELECT `+interruptColumns+` FROM interrupts WHERE root_run_id = ? AND state = ?`,
		runID, interruptStateOpen.databaseValue())
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return InterruptRecord{}, false, nil
	}
	if err != nil {
		return InterruptRecord{}, false, err
	}
	return p, true, nil
}

// Consume atomically reads AND deletes the pending interrupt for runID (one
// DELETE ... RETURNING), or returns ok=false when none is recorded — the resume
// claim contract. A single statement means two concurrent resumes can't both
// observe the same open interrupt: one claims it, the other gets ok=false, so a
// non-idempotent tool never re-fires.
func (i *InterruptStore) Consume(ctx context.Context, sessionID, runID string) (InterruptRecord, bool, error) {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return InterruptRecord{}, false, fmt.Errorf("sqlite: consume interrupt: %w", err)
	}
	row := conn(ctx, i.db).QueryRowContext(ctx,
		`DELETE FROM interrupts WHERE session_id = ? AND root_run_id = ? AND state = ?
		 RETURNING `+interruptColumns,
		sessionID, runID, interruptStateOpen.databaseValue())
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		if rejectForeignPendingOwnerErr := i.rejectForeignPendingOwner(ctx, sessionID, runID); rejectForeignPendingOwnerErr != nil {
			return InterruptRecord{}, false, rejectForeignPendingOwnerErr
		}
		return InterruptRecord{}, false, nil
	}
	if err != nil {
		return InterruptRecord{}, false, err
	}
	return p, true, nil
}

// ClaimResume atomically changes one exact open hand-off into a nonrecoverable
// resuming record while retaining the validated answer for audit and crash
// diagnosis. Open reads exclude the row until a new waiting boundary replaces
// it or terminal cleanup deletes it.
func (i *InterruptStore) ClaimResume(
	ctx context.Context,
	sessionID, runID string,
) (InterruptRecord, bool, error) {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return InterruptRecord{}, false, fmt.Errorf("sqlite: claim resume: %w", err)
	}
	row := conn(ctx, i.db).QueryRowContext(ctx,
		`UPDATE interrupts
		    SET state = ?
		  WHERE session_id = ? AND root_run_id = ? AND state = ?
		  RETURNING `+interruptColumns,
		interruptStateResuming.databaseValue(),
		sessionID, runID,
		interruptStateOpen.databaseValue(),
	)
	record, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		if rejectForeignPendingOwnerErr := i.rejectForeignPendingOwner(ctx, sessionID, runID); rejectForeignPendingOwnerErr != nil {
			return InterruptRecord{}, false, rejectForeignPendingOwnerErr
		}
		return InterruptRecord{}, false, nil
	}
	if err != nil {
		return InterruptRecord{}, false, err
	}
	return record, true, nil
}

// RequireResumeClaim proves that the exact root hand-off crossed the answer
// claim linearization point before its Run tree is reopened.
func (i *InterruptStore) RequireResumeClaim(ctx context.Context, sessionID, runID string) error {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return fmt.Errorf("sqlite: require resume claim: %w", err)
	}
	var owner string
	var state interruptState
	err := conn(ctx, i.db).QueryRowContext(ctx,
		`SELECT session_id, state FROM interrupts WHERE root_run_id = ?`, runID,
	).Scan(&owner, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("sqlite: resume claim does not exist")
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect resume claim: %w", err)
	}
	if owner != sessionID {
		return fmt.Errorf(
			"%w: Pending root Run %q belongs to Session %q, not %q",
			transcript.ErrIdentityConflict,
			runID,
			owner,
			sessionID,
		)
	}
	if state != interruptStateResuming {
		return fmt.Errorf("sqlite: interrupt for root Run %q is %q, not %s", runID, state, interruptStateResuming)
	}
	return nil
}

func (i *InterruptStore) Delete(ctx context.Context, sessionID, runID string) error {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	result, err := conn(ctx, i.db).ExecContext(ctx,
		`DELETE FROM interrupts WHERE session_id = ? AND root_run_id = ?`, sessionID, runID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect deleted interrupt: %w", err)
	}
	if deleted == 1 {
		return nil
	}
	return i.rejectForeignPendingOwner(ctx, sessionID, runID)
}

// DeleteResumeClaim consumes only the answer claim owned by a failed Resume.
// It leaves ordinary open reads unchanged and cannot delete a replacement open
// barrier that reuses the same root Run identity.
func (i *InterruptStore) DeleteResumeClaim(
	ctx context.Context,
	sessionID, runID, rootMemberID string,
) error {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return fmt.Errorf("sqlite: delete Resume claim: %w", err)
	}
	if err := runtimeidentity.ValidateMember(rootMemberID); err != nil {
		return fmt.Errorf("sqlite: delete Resume claim: %w", err)
	}
	result, err := conn(ctx, i.db).ExecContext(ctx,
		`DELETE FROM interrupts
		  WHERE session_id = ? AND root_run_id = ? AND root_member_id = ? AND state = ?`,
		sessionID, runID, rootMemberID, interruptStateResuming.databaseValue(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete Resume claim: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect deleted Resume claim: %w", err)
	}
	if deleted == 1 {
		return nil
	}
	if err := i.rejectForeignPendingOwner(ctx, sessionID, runID); err != nil {
		return err
	}
	return fmt.Errorf("sqlite: matching resuming interrupt for root Run %q was not found", runID)
}

func validatePendingOwner(sessionID, rootRunID string) error {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return err
	}
	if err := resourceid.ValidateRun(rootRunID); err != nil {
		return err
	}
	return nil
}

func (i *InterruptStore) rejectForeignPendingOwner(ctx context.Context, sessionID, rootRunID string) error {
	var owner string
	err := conn(ctx, i.db).QueryRowContext(ctx,
		`SELECT session_id FROM interrupts WHERE root_run_id = ?`,
		rootRunID,
	).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect interrupt %q owner: %w", rootRunID, err)
	}
	return fmt.Errorf(
		"%w: Pending root Run %q belongs to Session %q, not %q",
		transcript.ErrIdentityConflict,
		rootRunID,
		owner,
		sessionID,
	)
}

// scanRow abstracts *sql.Row and *sql.Rows so one scan path serves Get +
// List.
func scanPending(row scanRow) (InterruptRecord, error) {
	var (
		p               InterruptRecord
		payload         string
		rootMemberID    string
		continuations   string
		encodedBindings string
		createdNs       int64
	)
	if err := row.Scan(
		&p.RootRunID,
		&p.SessionID,
		&p.ExecutorID,
		&rootMemberID,
		&payload,
		&continuations,
		&encodedBindings,
		&createdNs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return InterruptRecord{}, err
		}
		return InterruptRecord{}, fmt.Errorf("sqlite: scan interrupt: %w", err)
	}
	var err error
	if p.Interrupts, err = decodeInterrupts(payload); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupts: %w", err)
	}
	var continuationValues []continuationRow
	if decodeInterruptJSONErr := decodeStoredJSON([]byte(continuations), &continuationValues); decodeInterruptJSONErr != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", decodeInterruptJSONErr)
	}
	p.Continuations = continuationsFromRows(continuationValues)
	var bindingValues []interruptBindingRow
	if decodeInterruptJSONErr := decodeStoredJSON([]byte(encodedBindings), &bindingValues); decodeInterruptJSONErr != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode input-request bindings: %w", decodeInterruptJSONErr)
	}
	p.Bindings = interruptBindingsFromRows(bindingValues)
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	if err := p.validateStorageShape(); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt %q: %w", p.RootRunID, err)
	}
	root, _ := p.rootContinuation()
	if root.MemberID != rootMemberID {
		return InterruptRecord{}, fmt.Errorf(
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
func decodeInterrupts(payload string) ([]OpenInterruptRecord, error) {
	if payload == "" {
		return nil, nil
	}
	var rows []interruptPayload
	if err := decodeStoredJSON([]byte(payload), &rows); err != nil {
		return nil, err
	}
	return interruptsFromPayloads(rows)
}

func drainedToolRows(tools []DrainedToolRecord) []drainedToolRow {
	rows := make([]drainedToolRow, len(tools))
	for index, tool := range tools {
		rows[index] = drainedToolRow(tool)
	}
	return rows
}

func drainedToolsFromRows(rows []drainedToolRow) []DrainedToolRecord {
	tools := make([]DrainedToolRecord, len(rows))
	for index, row := range rows {
		tools[index] = DrainedToolRecord(row)
	}
	return tools
}

func continuationRows(values []ContinuationRecord) []continuationRow {
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

func continuationsFromRows(rows []continuationRow) []ContinuationRecord {
	values := make([]ContinuationRecord, len(rows))
	for index, row := range rows {
		values[index] = ContinuationRecord{
			RunID:        row.RunID,
			MemberID:     row.MemberID,
			DrainedTools: drainedToolsFromRows(row.DrainedTools),
		}
	}
	return values
}

func interruptPayloads(values []OpenInterruptRecord) []interruptPayload {
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

func interruptsFromPayloads(rows []interruptPayload) ([]OpenInterruptRecord, error) {
	values := make([]OpenInterruptRecord, len(rows))
	for index, row := range rows {
		values[index] = OpenInterruptRecord{ItemID: row.ItemID}
		if review := row.Approval; review != nil {
			risk := tool.RiskLevel(review.Risk)
			if !risk.Valid() {
				return nil, fmt.Errorf("interrupt[%d] approval has unknown risk %q", index, review.Risk)
			}
			values[index].Approval = &ApprovalReviewRecord{
				Risk: risk, Reason: review.Reason, Rememberable: review.Rememberable,
			}
		}
	}
	return values, nil
}

func interruptBindingRows(values []InterruptBindingRecord) []interruptBindingRow {
	rows := make([]interruptBindingRow, len(values))
	for index, value := range values {
		rows[index] = interruptBindingRow(value)
	}
	return rows
}

func interruptBindingsFromRows(rows []interruptBindingRow) []InterruptBindingRecord {
	values := make([]InterruptBindingRecord, len(rows))
	for index, row := range rows {
		values[index] = InterruptBindingRecord(row)
	}
	return values
}
