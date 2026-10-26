package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestMergeSessionDraftPreservesBothAuthoringValues(t *testing.T) {
	shared := authoring.Attachment{
		ID: "shared", Kind: protocol.ContentBlockText, Name: "shared.txt", Path: "/workspace/shared.txt", Size: 10,
	}
	existing := authoring.Message{
		Text: "destination draft",
		Attachments: []authoring.Attachment{
			shared,
			{ID: "destination", Kind: protocol.ContentBlockText, Name: "destination.txt", Path: "/workspace/destination.txt", Size: 20},
		},
	}
	incoming := authoring.Message{
		Text: "input authored during navigation",
		Attachments: []authoring.Attachment{
			shared,
			{ID: "incoming", Kind: protocol.ContentBlockText, Name: "incoming.txt", Path: "/workspace/incoming.txt", Size: 30},
		},
	}
	wantExisting, wantIncoming := existing.Clone(), incoming.Clone()

	merged, err := MergeSessionDraft(existing, incoming)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Text != "destination draft\n\ninput authored during navigation" || len(merged.Attachments) != 3 {
		t.Fatalf("merged draft = %+v", merged)
	}
	if !existing.Equal(wantExisting) || !incoming.Equal(wantIncoming) {
		t.Fatal("MergeSessionDraft mutated an input value")
	}
}

func TestStoreRecoversSessionDraftTransferAfterPartialCommit(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := DraftTransfer{
		SourceSessionID:      "source",
		DestinationSessionID: "destination",
		SourceBefore:         authoring.Message{Text: "latest source edit"},
		SourceAfter:          authoring.Message{Text: "source baseline"},
		DestinationBefore:    authoring.Message{Text: "destination draft"},
		DestinationAfter:     authoring.Message{Text: "latest source edit"},
	}
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if saveDraftErr := store.SaveDraft(transfer.DestinationSessionID, transfer.DestinationBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}

	sourcePath := filepath.Join(directory, store.sessionStateName(transfer.SourceSessionID))
	backupPath := sourcePath + ".backup"
	if renameErr := os.Rename(sourcePath, backupPath); renameErr != nil {
		t.Fatal(renameErr)
	}
	if mkdirErr := os.Mkdir(sourcePath, 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeFileErr := os.WriteFile(filepath.Join(sourcePath, "blocker"), []byte("block replacement"), 0o600); writeFileErr != nil {
		t.Fatal(writeFileErr)
	}

	if applyDraftTransferErr := store.ApplyDraftTransfer(transfer); applyDraftTransferErr == nil {
		t.Fatal("partially blocked draft transfer unexpectedly succeeded")
	}
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, authoring.Message{Text: "must not overwrite the journal"}); saveDraftErr == nil ||
		!strings.Contains(saveDraftErr.Error(), "draft transfer") {
		t.Fatalf("source mutation while transfer is pending = %v", saveDraftErr)
	}
	if destination, found := store.Draft(transfer.DestinationSessionID); !found ||
		!destination.Equal(transfer.DestinationAfter) {
		t.Fatalf("partially committed destination = %+v, found %t", destination, found)
	}

	if removeErr := os.Remove(filepath.Join(sourcePath, "blocker")); removeErr != nil {
		t.Fatal(removeErr)
	}
	if removeErr := os.Remove(sourcePath); removeErr != nil {
		t.Fatal(removeErr)
	}
	if renameErr := os.Rename(backupPath, sourcePath); renameErr != nil {
		t.Fatal(renameErr)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	assertDraft(t, reopened, transfer.SourceSessionID, transfer.SourceAfter)
	assertDraft(t, reopened, transfer.DestinationSessionID, transfer.DestinationAfter)
	if _, err := os.Stat(filepath.Join(directory, sessionDraftTransferName)); !os.IsNotExist(err) {
		t.Fatalf("draft transfer journal survived recovery: %v", err)
	}
}

func TestDraftTransferDoesNotNormalizeSessionIdentity(t *testing.T) {
	transfer := DraftTransfer{
		SourceSessionID:      " source",
		DestinationSessionID: "destination",
		SourceBefore:         authoring.Message{Text: "before"},
		SourceAfter:          authoring.Message{Text: "after"},
	}
	if err := transfer.validate(); err == nil {
		t.Fatal("DraftTransfer accepted an identity that requires trimming")
	}
}

func TestStoreRecoversRetiredSourceDraftWithoutDuplicatingOwnership(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := DraftTransfer{
		SourceSessionID:      "deleted",
		DestinationSessionID: "replacement",
		SourceBefore:         authoring.Message{Text: "move me"},
		DestinationAfter:     authoring.Message{Text: "move me"},
	}
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if saveErr := store.save(sessionDraftTransferName, transfer); saveErr != nil {
		t.Fatal(saveErr)
	}
	// Simulate a crash after the source retirement but before the destination
	// replacement. Recovery must finish the move instead of losing the draft.
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, authoring.Message{}); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	assertDraft(t, reopened, transfer.SourceSessionID, authoring.Message{})
	assertDraft(t, reopened, transfer.DestinationSessionID, transfer.DestinationAfter)
}

func TestStoreRefusesToReplayDraftTransferOverNewerAuthoringState(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := DraftTransfer{
		SourceSessionID:      "source",
		DestinationSessionID: "destination",
		SourceBefore:         authoring.Message{Text: "before"},
		SourceAfter:          authoring.Message{Text: "baseline"},
		DestinationAfter:     authoring.Message{Text: "before"},
	}
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if saveErr := store.save(sessionDraftTransferName, transfer); saveErr != nil {
		t.Fatal(saveErr)
	}
	newer := authoring.Message{Text: "authored after the stale journal"}
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, newer); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, openErr := OpenDirectory(directory, Config{}); openErr == nil || !strings.Contains(openErr.Error(), "source draft changed") {
		t.Fatalf("open with conflicting draft transfer = %v", openErr)
	}
	if removeErr := os.Remove(filepath.Join(directory, sessionDraftTransferName)); removeErr != nil {
		t.Fatal(removeErr)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	assertDraft(t, reopened, transfer.SourceSessionID, newer)
}

func assertDraft(t *testing.T, store *Store, sessionID string, want authoring.Message) {
	t.Helper()
	got, found := store.Draft(sessionID)
	if found != !want.IsEmpty() || !got.Equal(want) {
		t.Fatalf("draft %s = %+v, found %t; want %+v", sessionID, got, found, want)
	}
}
