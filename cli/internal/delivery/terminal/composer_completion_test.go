package terminal

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/attachment"
)

func TestFileCompletionCannotAcceptAnOfferForEarlierInput(t *testing.T) {
	resolver, err := attachment.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The unattached loop cannot apply the new lookup, so acceptance exercises
	// the interval between editing the query and receiving its candidates.
	a := &app{loop: &program.Runtime{}, operations: newOperationOwner(t.Context()), attachments: resolver}
	t.Cleanup(a.operations.Close)
	a.composer.Editor().SetText("@c")
	query, ok := a.currentCompletionQuery()
	if !ok {
		t.Fatal("file input did not open a completion query")
	}
	a.completion.Accept = func(_ headless.Candidate, token headless.Token) {
		t.Errorf("accepted obsolete replacement range %+v for @cache", token)
	}
	a.completion.Offer(query.token, []headless.Candidate{{Text: "cache_test.go"}})
	a.composer.Editor().SetText("@cache")
	a.refreshCompletion()
	if a.handleCompletion(input.Key{Code: input.Tab}) {
		t.Fatal("earlier file candidates consumed completion acceptance while the new lookup was pending")
	}
	if got := a.composer.Editor().Text(); got != "@cache" {
		t.Fatalf("pending completion changed the draft to %q", got)
	}
}

func TestCompletionGateKeepsARejectedTokenClosedUntilItChanges(t *testing.T) {
	t.Parallel()

	query := completionQuery{
		line:  2,
		token: headless.Token{Start: 9, End: 16, Query: "archive", Trigger: headless.Trigger{Prefix: "@"}},
	}
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
	changed.token.Query += "s"
	changed.token.End++
	if !gate.Allow(changed) || !gate.Allow(query) {
		t.Fatal("editing the token did not begin a new completion query")
	}
	gate.Suppress(query)
	gate.Reset()
	if !gate.Allow(query) {
		t.Fatal("reset did not release the suppressed query")
	}
}
