package terminal

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/kit"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/attachment"
)

func TestCompletionAcceptsTheCurrentMultilineToken(t *testing.T) {
	operations := newOperationOwner(t.Context())
	t.Cleanup(operations.Close)
	composer := testPromptComposer(t)
	completion := newPromptCompletion(promptCompletionConfig{
		composer: composer, tasks: unattachedTasks(operations),
		theme: kit.Dark(), glyphs: kit.Unicode(),
	})
	composer.Controller().SetText("preserve this line\n/hel")
	token, ok := completion.currentToken()
	if !ok || token.Line != 1 {
		t.Fatalf("multiline token = %+v, %v", token, ok)
	}
	completion.popup.Offer(token, []base.Candidate{{Text: "help"}})
	if !completion.handleCompletion(input.Key{Code: input.Enter}) {
		t.Fatal("current completion was not accepted")
	}
	if got := composer.Controller().Text(); got != "preserve this line\n/help" {
		t.Fatalf("completed draft = %q", got)
	}
}

func TestCompletionRejectsAnOfferAfterCursorMovesToAnIdenticalToken(t *testing.T) {
	composer := testPromptComposer(t)
	completion := newPromptCompletion(promptCompletionConfig{
		composer: composer, theme: kit.Dark(), glyphs: kit.Unicode(),
	})
	composer.Controller().SetText("/hel\n/hel")
	token, ok := completion.currentToken()
	if !ok {
		t.Fatal("missing initial completion token")
	}
	completion.popup.SetAccept(func(base.Candidate, base.Token) {
		t.Fatal("accepted a completion captured on another line")
	})
	completion.popup.Offer(token, []base.Candidate{{Text: "help"}})
	composer.Controller().SetCursor(0, len("/hel"))
	if completion.handleCompletion(input.Key{Code: input.Enter}) {
		t.Fatal("stale completion consumed the new line's Enter")
	}
	if got := composer.Controller().Text(); got != "/hel\n/hel" {
		t.Fatalf("stale completion changed the draft to %q", got)
	}
}

func TestFileCompletionCannotAcceptAnOfferForEarlierInput(t *testing.T) {
	resolver, err := attachment.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The unattached loop cannot apply the new lookup, so acceptance exercises
	// the interval between editing the query and receiving its candidates.
	operations := newOperationOwner(t.Context())
	t.Cleanup(operations.Close)
	composer := testPromptComposer(t)
	composer.attachments = resolver
	transcript := testTranscriptView(t)
	completion := newPromptCompletion(promptCompletionConfig{
		composer: composer, tasks: unattachedTasks(operations), commands: &commandCatalog{},
		theme: transcript.theme, glyphs: transcript.glyphs, notify: func(string) {},
	})
	a := struct {
		composer   *promptComposer
		completion *promptCompletion
	}{composer, completion}
	a.composer.Controller().SetText("@c")
	query, ok := a.completion.currentToken()
	if !ok {
		t.Fatal("file input did not open a completion query")
	}
	a.completion.popup.SetAccept(func(_ base.Candidate, token base.Token) {
		t.Errorf("accepted obsolete replacement range %+v for @cache", token)
	})
	a.completion.popup.Offer(query, []base.Candidate{{Text: "cache_test.go"}})
	a.composer.Controller().SetText("@cache")
	a.completion.refreshCompletion()
	if a.completion.handleCompletion(input.Key{Code: input.Tab}) {
		t.Fatal("earlier file candidates consumed completion acceptance while the new lookup was pending")
	}
	if got := a.composer.Controller().Text(); got != "@cache" {
		t.Fatalf("pending completion changed the draft to %q", got)
	}
}

func TestCompletionGateKeepsARejectedTokenClosedUntilItChanges(t *testing.T) {
	t.Parallel()

	query := base.Token{Line: 2, Start: 9, End: 16, Query: "archive", Trigger: base.Trigger{Prefix: "@"}}
	var gate completionGate
	if !gate.Allow(query) {
		t.Fatal("fresh completion query was suppressed")
	}
	gate.Suppress(query)
	if gate.Allow(query) {
		t.Fatal("rejected completion query reopened immediately")
	}
	if gate.Allow(query) {
		t.Fatal("rejected completion query reopened without an edit")
	}
	changed := query
	changed.Query += "s"
	changed.End++
	if !gate.Allow(changed) || !gate.Allow(query) {
		t.Fatal("editing the token did not begin a new completion query")
	}
	gate.Suppress(query)
	gate.Reset()
	if !gate.Allow(query) {
		t.Fatal("reset did not release the suppressed query")
	}
}
