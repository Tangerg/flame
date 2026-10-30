package terminal

import (
	"context"
	"sync"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/oolong/core/input"
)

type observedSessionUpdates struct {
	Runtime
	updates chan conversation.UpdateSession
	block   <-chan struct{}
}

func (r *observedSessionUpdates) UpdateSession(ctx context.Context, request conversation.UpdateSession) (conversation.Session, error) {
	r.updates <- request
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return conversation.Session{}, context.Cause(ctx)
		}
	}
	return r.Runtime.UpdateSession(ctx, request)
}

func TestSessionRenameBusyAdmissionKeepsDraftAndOriginalRevision(t *testing.T) {
	base := runtimefixture.New()
	base.Instant = true
	workspace := t.TempDir()
	original, err := base.CreateSession(t.Context(), conversation.CreateSession{Title: "Busy rename target", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	blocked := make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	runtime := &observedSessionUpdates{Runtime: base, updates: make(chan conversation.UpdateSession, 4), block: blocked}
	host, stop := runUIWithWorkspace(t, runtime, workspace)
	t.Cleanup(release)
	host.Shows(t, "Ask flame")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions · Center")
	host.Type(original.Title)
	host.Shows(t, original.Title)
	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	first := awaitValue(t, runtime.updates, "favorite mutation")
	if first.Favorite == nil || first.ExpectedRevision != original.Revision {
		t.Fatalf("favorite request = %+v", first)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Alt})
	host.Shows(t, "Rename session")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("Busy rename draft")
	host.Press(input.Enter)
	host.Shows(t, "Wait for the current Session action to finish.")
	host.Shows(t, "Busy rename draft")
	select {
	case request := <-runtime.updates:
		t.Fatalf("busy form originated another update: %+v", request)
	default:
	}
	release()
	host.Shows(t, "Favorites · "+original.Title)
	host.Press(input.Enter)
	request := awaitValue(t, runtime.updates, "original rename after favorite settlement")
	if request.Title == nil || *request.Title != "Busy rename draft" || request.ExpectedRevision != original.Revision {
		t.Fatalf("renaming drifted to the favorite revision: %+v", request)
	}
	host.Shows(t, "revision conflict")
	host.Shows(t, "Busy rename draft")
	host.Press(input.Esc)
	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionRenameUntouchedFormDoesNotOverwriteRemoteTitle(t *testing.T) {
	base := runtimefixture.New()
	base.Instant = true
	workspace := t.TempDir()
	original, err := base.CreateSession(t.Context(), conversation.CreateSession{Title: "Untouched rename target", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &observedSessionUpdates{Runtime: base, updates: make(chan conversation.UpdateSession, 4)}
	host, stop := runUIWithWorkspace(t, runtime, workspace)
	host.Shows(t, "Ask flame")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions · Center")
	host.Type(original.Title)
	host.Shows(t, original.Title)
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Alt})
	host.Shows(t, "Rename session")
	remoteTitle := "Remote untouched winner"
	remote, err := base.UpdateSession(t.Context(), conversation.UpdateSession{SessionID: original.ID, ExpectedRevision: original.Revision, Title: &remoteTitle})
	if err != nil {
		t.Fatal(err)
	}
	host.Press(input.Enter)
	host.Hides(t, "Rename session")
	select {
	case request := <-runtime.updates:
		t.Fatalf("untouched form submitted a write: %+v", request)
	default:
	}
	got, err := base.GetSession(t.Context(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Session.Title != remote.Title || got.Session.Revision != remote.Revision {
		t.Fatalf("remote title overwritten: %+v", got.Session)
	}
	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionRenamePreservesOpenedRevisionAndRefusedDraft(t *testing.T) {
	base := runtimefixture.New()
	base.Instant = true
	workspace := t.TempDir()
	original, err := base.CreateSession(t.Context(), conversation.CreateSession{Title: "Original rename target", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &observedSessionUpdates{Runtime: base, updates: make(chan conversation.UpdateSession, 4)}
	host, stop := runUIWithWorkspace(t, runtime, workspace)
	defer stop()
	host.Shows(t, "Ask flame")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions · Center")
	host.Type(original.Title)
	host.Shows(t, original.Title)
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Alt})
	host.Shows(t, "Rename session")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("Local rename draft")

	remoteTitle := "Remote rename won"
	remote, err := base.UpdateSession(t.Context(), conversation.UpdateSession{SessionID: original.ID, ExpectedRevision: original.Revision, Title: &remoteTitle})
	if err != nil {
		t.Fatal(err)
	}
	host.Press(input.Enter)
	request := awaitValue(t, runtime.updates, "rename submission")
	if request.ExpectedRevision != original.Revision || request.SessionID != original.ID || request.Title == nil || *request.Title != "Local rename draft" {
		t.Fatalf("rename request = %+v, want original revision %d and draft", request, original.Revision)
	}
	host.Shows(t, "revision conflict")
	host.Shows(t, "Rename session")
	host.Shows(t, "Local rename draft")
	got, err := base.GetSession(t.Context(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Session.Title != remoteTitle || got.Session.Revision != remote.Revision {
		t.Fatalf("refused edit changed remote Session: %+v", got.Session)
	}
	host.Press(input.Esc)
	host.Hides(t, "Rename session")
	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions · Center")
	host.Type(remoteTitle)
	host.Shows(t, remoteTitle)
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Alt})
	host.Shows(t, "Rename session")
	host.Shows(t, remoteTitle)
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("Explicit fresh rename")
	host.Press(input.Enter)
	next := awaitValue(t, runtime.updates, "fresh rename submission")
	if next.ExpectedRevision != remote.Revision || next.Title == nil || *next.Title != "Explicit fresh rename" {
		t.Fatalf("new edit did not capture the fresh Session: %+v", next)
	}
	host.Hides(t, "Rename session")
	host.Shows(t, "Explicit fresh rename")
	confirmed, err := base.GetSession(t.Context(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Session.Title != "Explicit fresh rename" || confirmed.Session.Revision != remote.Revision+1 {
		t.Fatalf("fresh edit did not commit once: %+v", confirmed.Session)
	}
	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
}
