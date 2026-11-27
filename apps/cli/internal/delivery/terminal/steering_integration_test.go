package terminal

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestAcceptedSteerSettlesEvenWhenTheFollowingDraftCannotBeSaved(t *testing.T) {
	provider, entered, _ := openSteeringModel(t)
	connection, source := openTerminalIntegrationRuntime(t, provider)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	observed := make(chan protocol.SteerRunResponse, 1)
	release := make(chan struct{})
	backend := &heldTerminalSteerReceipt{Connection: connection, accepted: observed, release: release}
	directory := t.TempDir()
	config := sessionNavigationConfig(connection, backend, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, currentStore := runUIFromConfig(t, config)
	defer stop()
	host.Shows(t, "Ask flame")
	if !host.Type("start the held model call") || !host.Press(input.Enter) {
		t.Fatal("terminal refused the opening prompt")
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("Runtime did not enter the loopback provider", context.Cause(ctx))
	}
	host.Shows(t, "holding the first model call")
	const instruction = "remember the accepted steering instruction"
	if !host.Type("/steer "+instruction) || !host.Press(input.Enter) {
		t.Fatal("terminal refused the steering command")
	}
	select {
	case receipt := <-observed:
		if receipt.UserItemID == "" {
			t.Fatal("Runtime accepted steering without its user item identity")
		}
	case <-ctx.Done():
		t.Fatal("Runtime did not accept steering", context.Cause(ctx))
	}
	if _, pending := currentStore().PendingSteer(source.ID); !pending {
		t.Fatal("the original steer was never staged in the Workbench")
	}
	const draft = "following draft whose write is refused"
	database := inspectSessionWorkbench(t, directory)
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER refuse_following_draft
		BEFORE INSERT ON authoring_records
		WHEN new.key GLOB 'session/*' AND instr(CAST(new.body AS TEXT), 'following draft whose write is refused') > 0
		BEGIN SELECT RAISE(ABORT, 'following draft refused'); END`); err != nil {
		t.Fatal(err)
	}
	if !host.Type(draft) {
		t.Fatal("terminal refused the following draft")
	}
	host.Shows(t, draft)
	host.Shows(t, "workbench: commit workbench state")
	close(release)
	waitForSteeringState(t, ctx, "the accepted steer to leave the Workbench journal", func() (bool, error) {
		_, pending := currentStore().PendingSteer(source.ID)
		return !pending, nil
	})
	host.Shows(t, draft)
	if _, err := database.ExecContext(ctx, "DROP TRIGGER refuse_following_draft"); err != nil {
		t.Fatal(err)
	}
	stop()
	reopened, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, pending := reopened.PendingSteer(source.ID); pending {
		t.Fatal("accepted steer is still pending after reopening")
	}
	if !slices.ContainsFunc(reopened.History(), func(message authoring.Message) bool { return message.Text == instruction }) {
		t.Fatal("the accepted steering instruction is missing from history")
	}
	if saved, _ := reopened.Draft(source.ID); saved.Text != draft {
		t.Fatalf("following draft = %q, want %q", saved.Text, draft)
	}
}

type heldTerminalSteerReceipt struct {
	*runtimebinding.Connection
	accepted chan<- protocol.SteerRunResponse
	release  <-chan struct{}
}

func (r *heldTerminalSteerReceipt) SteerRun(ctx context.Context, input authoring.SteerRun) (protocol.SteerRunResponse, error) {
	receipt, err := r.Connection.SteerRun(ctx, input)
	if err != nil {
		return receipt, err
	}
	r.accepted <- receipt
	select {
	case <-r.release:
		return receipt, nil
	case <-ctx.Done():
		return receipt, context.Cause(ctx)
	}
}
