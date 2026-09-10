package workbench

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

func TestPendingSteerAtomicallyReturnsAttachmentsIntoANewerDraft(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "ses_steer"
	attachment := steerTestAttachment(t.TempDir())
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	if saveDraftErr := store.SaveDraft(sessionID, source); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	pending := steerTestPending(t, sessionID, attachment)
	if stagePendingSteerErr := store.StagePendingSteer(pending, source); stagePendingSteerErr != nil {
		t.Fatal(stagePendingSteerErr)
	}
	if draft, found := store.Draft(sessionID); found {
		t.Fatalf("draft after staging = %+v, found %t", draft, found)
	}

	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	staged, found := reopened.PendingSteer(sessionID)
	if !found || !staged.Command().Equal(pending.Command()) {
		t.Fatalf("reopened pending steer = %+v, found %t", staged, found)
	}
	newer := agent.Message{Text: "new input while steer settles"}
	if saveDraftErr := reopened.SaveDraft(sessionID, newer); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	recovered, err := reopened.RejectPendingSteer(sessionID, pending.CommandID(), newer)
	if err != nil {
		t.Fatal(err)
	}
	want := agent.Message{Text: newer.Text, Attachments: []agent.Attachment{attachment}}
	if !recovered.Equal(want) {
		t.Fatalf("recovered draft = %+v, want %+v", recovered, want)
	}

	settled, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := settled.PendingSteer(sessionID); found {
		t.Fatal("rejected pending steer survived restart")
	}
	if draft, found := settled.Draft(sessionID); !found || !draft.Equal(want) {
		t.Fatalf("settled draft = %+v, found %t", draft, found)
	}
}

func TestPendingSteerAcknowledgementIsRestartIdempotentAndPreservesDraft(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "ses_steer"
	attachment := steerTestAttachment(t.TempDir())
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	pending := steerTestPending(t, sessionID, attachment)
	if saveDraftErr := store.SaveDraft(sessionID, source); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if stagePendingSteerErr := store.StagePendingSteer(pending, source); stagePendingSteerErr != nil {
		t.Fatal(stagePendingSteerErr)
	}
	newer := agent.Message{Text: "keep this newer thought"}
	if saveDraftErr := store.SaveDraft(sessionID, newer); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if acknowledgePendingSteerErr := store.AcknowledgePendingSteer(sessionID, pending.CommandID()); acknowledgePendingSteerErr != nil {
		t.Fatal(acknowledgePendingSteerErr)
	}

	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := reopened.PendingSteer(sessionID); found {
		t.Fatal("acknowledged pending steer survived restart")
	}
	if draft, found := reopened.Draft(sessionID); !found || !draft.Equal(newer) {
		t.Fatalf("newer draft = %+v, found %t", draft, found)
	}
	history := reopened.History()
	if len(history) != 1 || !history[0].Equal(pending.Message()) {
		t.Fatalf("steer history = %+v", history)
	}
}

func TestPendingSteerRejectsInstructionThatDoesNotExactlyOwnTheSourceDraft(t *testing.T) {
	sessionID := "ses_steer"
	pending := steerTestPending(t, sessionID, steerTestAttachment(t.TempDir()))
	command := pending.Command()
	command.Message.Attachments = nil
	command.Message.Text = " inspect the parser "
	if _, err := NewPendingSteer(sessionID, command, pending.StagedAt(), pending.Replay()); err == nil {
		t.Fatal("non-canonical steer instruction claimed ownership of a different source draft")
	}
}

func TestPendingSteerOwnsDetachedCommandMaterial(t *testing.T) {
	pending := steerTestPending(t, "ses_steer", steerTestAttachment(t.TempDir()))
	command := pending.Command()
	command.Message.Text = "mutated"
	command.Message.Attachments[0].Name = "mutated.txt"
	if pending.Message().Text != "inspect the parser" || pending.Message().Attachments[0].Name != "notes.txt" {
		t.Fatal("caller mutation changed durable steer ownership")
	}
}

func TestPendingSteerSpellsOneStagingTimeWhateverZoneItArrivesIn(t *testing.T) {
	utc := steerTestPending(t, "ses_steer", steerTestAttachment(t.TempDir()))
	zoned, err := NewPendingSteer(
		utc.SessionID(),
		utc.Command(),
		utc.StagedAt().In(time.FixedZone("east", 8*60*60)),
		utc.Replay(),
	)
	if err != nil {
		t.Fatal(err)
	}
	// The staging time is persisted through pendingSteerRecord, and time.Time
	// keeps its zone through JSON. One instant must be one durable record.
	utcJSON, err := json.Marshal(utc.record())
	if err != nil {
		t.Fatal(err)
	}
	zonedJSON, err := json.Marshal(zoned.record())
	if err != nil {
		t.Fatal(err)
	}
	if string(zonedJSON) != string(utcJSON) {
		t.Fatalf("steer record = %s, want %s", zonedJSON, utcJSON)
	}
}

func steerTestAttachment(directory string) agent.Attachment {
	return agent.Attachment{
		ID: "att_notes", Kind: protocol.ContentBlockText, Name: "notes.txt",
		Path: filepath.Join(directory, "notes.txt"), MimeType: "text/plain", Size: 5,
	}
}

func steerTestPending(
	t *testing.T,
	sessionID string,
	attachment agent.Attachment,
) PendingSteer {
	t.Helper()
	stagedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	replay, err := commandreplay.NewProtectedGuard("runtime-test", stagedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := NewPendingSteer(
		sessionID,
		agent.SteerRun{
			CommandID: "cli_11111111111111111111111111111111",
			RunID:     "run_1", SegmentID: "seg_1",
			Message: agent.Message{Text: "inspect the parser", Attachments: []agent.Attachment{attachment}},
		},
		stagedAt,
		replay,
	)
	if err != nil {
		t.Fatal(err)
	}
	return pending
}

// TestSteerSettlementRefusesAnotherCommandsSteer pins the half of the claim the
// existing tests never exercised: a settlement naming a steer that some newer
// command already replaced. The stale identity has to be well-formed, or the
// shape check refuses it first and the claim is never asked.
func TestSteerSettlementRefusesAnotherCommandsSteer(t *testing.T) {
	store, err := OpenDirectory(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "ses_steer_identity"
	attachment := steerTestAttachment(t.TempDir())
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	if err := store.SaveDraft(sessionID, source); err != nil {
		t.Fatal(err)
	}
	pending := steerTestPending(t, sessionID, attachment)
	if err := store.StagePendingSteer(pending, source); err != nil {
		t.Fatal(err)
	}

	stale := agent.CommandID("cli_22222222222222222222222222222222")
	if err := stale.Validate(); err != nil {
		t.Fatalf("the stale identity is malformed, so the claim would never be asked: %v", err)
	}
	if stale == pending.CommandID() {
		t.Fatal("the stale identity is the staged one")
	}
	if err := store.AcknowledgePendingSteer(sessionID, stale); err == nil {
		t.Fatal("acknowledgement settled a steer it does not name")
	}
	if _, err := store.RejectPendingSteer(sessionID, stale, agent.Message{}); err == nil {
		t.Fatal("rejection settled a steer it does not name")
	}
	if _, found := store.PendingSteer(sessionID); !found {
		t.Fatal("a refused settlement discarded the steer that is still pending")
	}
}
