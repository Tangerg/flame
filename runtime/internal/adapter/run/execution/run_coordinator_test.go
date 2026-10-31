package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	corechat "github.com/Tangerg/scope/core/chat"
)

// inertApprovalRulePreparer derives a rule without a source-authority check so an
// execution coordinator test exercises acceptance without a live approval policy.
type inertApprovalRulePreparer struct{}

func (inertApprovalRulePreparer) PrepareRule(_ context.Context, request approval.RememberRequest) (approval.Rule, error) {
	return request.Rule()
}

type inertAcceptedContinuationStore struct{}

func (inertAcceptedContinuationStore) List(context.Context) ([]runs.AcceptedContinuation, error) {
	return nil, nil
}

func (inertAcceptedContinuationStore) Delete(context.Context, string) error { return nil }

type passthroughRunWorkingContext struct{}

func (passthroughRunWorkingContext) ComposeWorkingContext(
	_ context.Context,
	input runs.WorkingContextInput,
) ([]corechat.Message, error) {
	return input.Seed, nil
}

type acceptAllRunModelInputs struct{}

func (acceptAllRunModelInputs) AdmitSelection(context.Context, modelchoice.Selection) error {
	return nil
}

// runTestProjection names one projection fake per port; the Coordinator and
// its segment supervisor each take the ports they consume.
type runTestProjection struct {
	Openings                    runs.OpeningCommitter
	ChildStarts                 runs.ChildRunStartCommitter
	Events                      runs.EventCommitter
	Barriers                    runs.TreeBarrierCommitter
	Checkpoints                 runs.WaitingCheckpointReader
	WaitingSubtreeCancellations runs.WaitingSubtreeCancellationCommitter
	Workspace                   runs.WorkspaceChangeNotifier
	Finalizer                   runs.SegmentFinalizer
}

// runTestDeps fixes one collaborator set for the run use cases and the
// segment supervisor they open segments on.
type runTestDeps struct {
	runs.Dependencies
	// The collaborators only opening or cancellation uses; shared ones come
	// from Dependencies.
	// Session is one double for every Session-side port.
	Session                            any
	RootStarts                         runs.RootExecutionStarter
	Releases                           runs.ExecutionReleaser
	Conversation                       runs.ConversationReader
	RootCancellation                   runs.RunningRootCancellationRequester
	RunningSubtreeCanceler             runs.RunningSubtreeCanceler
	WaitingSubtreeCancellationPreparer runs.WaitingSubtreeCancellationPreparer
	Observations                       runs.ExecutionObserver
	Projection                         runTestProjection
	Now                                func() time.Time
	Retention                          runs.Retention
	NewRunID                           func() string
	NewSegmentID                       func() string
}

// testRuns is one fixture over the run use cases and their supervisor.
type testRuns struct {
	*runs.Coordinator
	*runs.Openings
	*runs.Cancellations
	*runs.Segments
}

func mustNewRunCoordinator(t *testing.T, d runTestDeps) testRuns {
	t.Helper()
	deps := d.Dependencies
	opening := runs.OpeningDependencies{
		RootStarts: d.RootStarts, Releases: d.Releases, Conversation: d.Conversation,
		WorkingContexts: passthroughRunWorkingContext{}, Models: acceptAllRunModelInputs{},
	}
	if projection, ok := d.Projection.Openings.(*delegateProjection); ok {
		deps.CommandAcceptances = projection.stores.CommandAcceptances
		deps.AcceptedContinuations = projection.stores.AcceptedContinuations
		opening.Isolation = projection.isolation
	}
	if deps.CommandAcceptances == nil {
		deps.CommandAcceptances = noCommandAcceptances{}
	}
	if deps.ApprovalRules == nil {
		deps.ApprovalRules = inertApprovalRulePreparer{}
	}
	if deps.AcceptedContinuations == nil {
		deps.AcceptedContinuations = inertAcceptedContinuationStore{}
	}
	starter := any(d.RootStarts)
	fillRunDependency(t, &deps.Continuation, starter, "waiting execution continuer")
	fillRunDependency(t, &deps.Steering, starter, "running execution steerer")
	cancellation := runs.CancellationDependencies{
		RootCancellation:                   d.RootCancellation,
		RunningSubtreeCanceler:             d.RunningSubtreeCanceler,
		WaitingSubtreeCancellationPreparer: d.WaitingSubtreeCancellationPreparer,
	}
	fillRunDependency(t, &cancellation.RootCancellation, starter, "root cancellation requester")
	fillRunDependency(t, &cancellation.RunningSubtreeCanceler, starter, "running subtree canceler")
	fillRunDependency(t, &cancellation.WaitingSubtreeCancellationPreparer, starter, "waiting subtree cancellation preparer")

	sessions := d.Session
	fillRunDependency(t, &deps.Sessions, sessions, "session reader")
	fillRunDependency(t, &deps.Interrupts, sessions, "pending interrupt reader")
	fillRunDependency(t, &opening.Sessions, sessions, "session reader")
	fillRunDependency(t, &opening.SessionCreator, sessions, "session creator")
	fillRunDependency(t, &opening.ActiveRuns, sessions, "active run reader")
	fillRunDependency(t, &cancellation.Terminations, sessions, "run termination committer")
	ports := d.Projection
	projection := any(ports.Openings)
	fillRunDependency(t, &ports.ChildStarts, projection, "child run start committer")
	fillRunDependency(t, &ports.Events, projection, "event committer")
	fillRunDependency(t, &ports.Barriers, projection, "tree barrier committer")
	fillRunDependency(t, &ports.Checkpoints, projection, "waiting checkpoint reader")
	if ports.WaitingSubtreeCancellations == nil {
		if value, ok := projection.(runs.WaitingSubtreeCancellationCommitter); ok {
			ports.WaitingSubtreeCancellations = value
		} else {
			ports.WaitingSubtreeCancellations = inertRunProjection{}
		}
	}
	fillRunDependency(t, &ports.Workspace, projection, "workspace notifier")
	fillRunDependency(t, &ports.Finalizer, projection, "segment finalizer")
	fillRunDependency(t, &deps.Runs, projection, "run projection")
	fillRunDependency(t, &deps.Items, projection, "item projection")
	deps.Checkpoints = ports.Checkpoints
	now := d.Now
	if now == nil {
		now = time.Now
	}
	retention := d.Retention
	if retention == (runs.Retention{}) {
		retention = runs.DefaultRetention()
	}
	segments, err := runs.NewSegments(runs.SegmentDependencies{
		Observations: d.Observations, Releases: d.Releases,
		Publication: runs.PublicationPorts{Openings: ports.Openings, Events: ports.Events, Barriers: ports.Barriers, Workspace: ports.Workspace},
		ChildStarts: ports.ChildStarts, Finalizer: ports.Finalizer, Runs: deps.Runs, Admissions: deps.Admissions,
		Now: now, Retention: retention, NewRunID: d.NewRunID, NewSegmentID: d.NewSegmentID,
	})
	if err != nil {
		t.Fatalf("construct Run segments: %v", err)
	}
	deps.Segments = segments
	coordinator, err := runs.NewCoordinator(deps)
	if err != nil {
		t.Fatalf("construct Run coordinator: %v", err)
	}
	fillRunDependency(t, &cancellation.WaitingRestorer, starter, "waiting execution restorer")
	cancellation.WaitingSubtreeCancellations = ports.WaitingSubtreeCancellations
	cancellation.Sessions, cancellation.Interrupts = deps.Sessions, deps.Interrupts
	cancellation.Checkpoints, cancellation.Releases = deps.Checkpoints, d.Releases
	cancellation.Segments, cancellation.Runs, cancellation.Items, cancellation.Admissions = segments, deps.Runs, deps.Items, deps.Admissions
	cancellations, err := runs.NewCancellations(cancellation)
	if err != nil {
		t.Fatalf("construct Run cancellations: %v", err)
	}
	opening.CommandAcceptances = deps.CommandAcceptances
	opening.Segments, opening.Admissions = segments, deps.Admissions
	openings, err := runs.NewOpenings(opening)
	if err != nil {
		t.Fatalf("construct Run openings: %v", err)
	}
	t.Cleanup(func() {
		segments.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := segments.AwaitShutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return testRuns{Coordinator: coordinator, Openings: openings, Cancellations: cancellations, Segments: segments}
}

func testDefaultSelection() modelchoice.Selection {
	return testsupport.MustModelSelection("test-provider", "test-model")
}

type inertRunProjection struct{}

func (inertRunProjection) CommitWaitingSubtreeCancellation(
	context.Context,
	runs.WaitingSubtreeCancellationCommit,
) (runs.WaitingSubtreeCancellationResult, error) {
	return runs.WaitingSubtreeCancellationResult{}, errors.New("inert Run projection")
}

func fillRunDependency[T any](t *testing.T, target *T, source any, name string) {
	t.Helper()
	if any(*target) != nil {
		return
	}
	value, ok := source.(T)
	if !ok {
		t.Fatalf("test fixture source does not implement %s", name)
	}
	*target = value
}

type noCommandAcceptances struct{}

func (noCommandAcceptances) Lookup(context.Context, string) (runs.CommandAcceptanceRecord, error) {
	return runs.CommandAcceptanceRecord{}, nil
}
