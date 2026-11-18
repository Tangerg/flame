package runs

import (
	"context"
	"errors"

	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	corechat "github.com/Tangerg/scope/core/chat"
)

type passthroughWorkingContext struct{}

func (passthroughWorkingContext) ComposeWorkingContext(
	_ context.Context,
	input WorkingContextInput,
) ([]corechat.Message, error) {
	return input.Seed, nil
}

type acceptAllModelInputs struct{}

func (acceptAllModelInputs) AdmitSelection(context.Context, modelchoice.Selection) error { return nil }

// mustNewCoordinator completes dependencies unrelated to a focused test. The
// production constructor remains strict; tests opt into explicit inert doubles
// instead of manufacturing partially valid Coordinators.
func mustNewCoordinator(deps runDeps) *testRuns {
	return mustNewCoordinatorWithSegments(deps, SegmentDependencies{})
}

// testRuns is the run use cases a test drives, built over the same
// collaborators and Segments.
type testRuns struct {
	*Coordinator
	openings      *Openings
	cancellations *Cancellations
}

func (t *testRuns) Start(ctx context.Context, cmd StartCommand) (StartResult, error) {
	return t.openings.Start(ctx, cmd)
}

func (t *testRuns) AdmitSelection(ctx context.Context, selection modelchoice.Selection) error {
	return t.openings.AdmitSelection(ctx, selection)
}

func (t *testRuns) WaitSessionStartable(ctx context.Context, sessionID string) error {
	return t.openings.WaitSessionStartable(ctx, sessionID)
}

func (t *testRuns) Cancel(ctx context.Context, cmd CancelCommand) (CancelResult, error) {
	return t.cancellations.Cancel(ctx, cmd)
}

// runDeps is every collaborator of the run use cases a test may fix. The
// helpers split it into each use case's own dependencies, sharing the rest.
type runDeps struct {
	Dependencies
	// Session is one double for every Session-side port.
	Session                            completeTestSessionPorts
	RootStarts                         RootExecutionStarter
	Releases                           ExecutionReleaser
	Conversation                       ConversationReader
	WorkingContexts                    WorkingContextComposer
	Models                             modelAdmitter
	ModelSelector                      modelapp.Selector
	Isolation                          isolationProvider
	RootCancellation                   RunningRootCancellationRequester
	WaitingRestorer                    waitingExecutionRestorer
	RunningSubtreeCanceler             RunningSubtreeCanceler
	WaitingSubtreeCancellationPreparer WaitingSubtreeCancellationPreparer
	WaitingSubtreeCancellations        WaitingSubtreeCancellationCommitter
	Terminations                       terminationCommitter
}

// mustNewCoordinatorWithSegments also fixes the segment supervisor's
// collaborators; defaults are shared with the use cases' own.
func mustNewCoordinatorWithSegments(deps runDeps, seg SegmentDependencies) *testRuns {
	deps = completeTestDependencies(deps, seg)
	coordinator, err := NewCoordinator(deps.Dependencies)
	if err != nil {
		panic(err)
	}
	openings, err := NewOpenings(OpeningDependencies{
		CommandAcceptances: deps.CommandAcceptances, RootStarts: deps.RootStarts, Releases: deps.Releases,
		Conversation: deps.Conversation, WorkingContexts: deps.WorkingContexts, Models: deps.Models,
		ModelSelector: deps.ModelSelector, Sessions: deps.Sessions, SessionCreator: deps.Session, ActiveRuns: deps.Session, Segments: deps.Segments,
		Admissions: deps.Admissions, Isolation: deps.Isolation,
	})
	if err != nil {
		panic(err)
	}
	cancellations, err := NewCancellations(CancellationDependencies{
		RootCancellation:                   deps.RootCancellation,
		WaitingRestorer:                    deps.WaitingRestorer,
		RunningSubtreeCanceler:             deps.RunningSubtreeCanceler,
		WaitingSubtreeCancellationPreparer: deps.WaitingSubtreeCancellationPreparer,
		WaitingSubtreeCancellations:        deps.WaitingSubtreeCancellations,
		Terminations:                       deps.Terminations,
		Sessions:                           deps.Sessions,
		Interrupts:                         deps.Interrupts,
		Checkpoints:                        deps.Checkpoints,
		Releases:                           deps.Releases,
		Segments:                           deps.Segments,
		Runs:                               deps.Runs,
		Items:                              deps.Items,
		Admissions:                         deps.Admissions,
	})
	if err != nil {
		panic(err)
	}
	return &testRuns{Coordinator: coordinator, openings: openings, cancellations: cancellations}
}

func completeTestDependencies(deps runDeps, seg SegmentDependencies) runDeps {
	control := &fakeExecutionPorts{}
	observer := &fakeExecutor{}
	sessions := &fakeRunSessions{}
	effects := &fakeEffects{}
	if deps.RootStarts == nil {
		deps.RootStarts = control
	}
	if deps.ModelSelector == nil {
		deps.ModelSelector = modelSelectorFunc(func(_ context.Context, input modelapp.SelectionInput) (modelchoice.Selection, error) {
			return input.Proposed, nil
		})
	}
	if deps.Isolation == nil {
		deps.Isolation = &stubIsolation{err: errors.New("isolation is not configured")}
	}
	if deps.Releases == nil {
		deps.Releases = control
	}
	if deps.Conversation == nil {
		deps.Conversation = emptyConversationReader{}
	}
	if deps.WorkingContexts == nil {
		deps.WorkingContexts = passthroughWorkingContext{}
	}
	if deps.Models == nil {
		deps.Models = acceptAllModelInputs{}
	}
	if deps.Continuation == nil {
		deps.Continuation = control
	}
	if deps.ApprovalRules == nil {
		deps.ApprovalRules = stubApprovalRulePreparer{}
	}
	if deps.AcceptedContinuations == nil {
		deps.AcceptedContinuations = stubAcceptedContinuationStore{}
	}
	if deps.Steering == nil {
		deps.Steering = control
	}
	if deps.RootCancellation == nil {
		deps.RootCancellation = control
	}
	if deps.WaitingRestorer == nil {
		deps.WaitingRestorer = control
	}
	if deps.RunningSubtreeCanceler == nil {
		deps.RunningSubtreeCanceler = control
	}
	if deps.WaitingSubtreeCancellationPreparer == nil {
		deps.WaitingSubtreeCancellationPreparer = control
	}
	if deps.Session == nil {
		deps.Session = sessions
	}
	if deps.Sessions == nil {
		deps.Sessions = deps.Session
	}
	if deps.Interrupts == nil {
		deps.Interrupts = deps.Session
	}
	if deps.Terminations == nil {
		deps.Terminations = deps.Session
	}
	if deps.Checkpoints == nil {
		deps.Checkpoints = effects
	}
	if deps.WaitingSubtreeCancellations == nil {
		deps.WaitingSubtreeCancellations = effects
	}
	if deps.CommandAcceptances == nil {
		deps.CommandAcceptances = noCommandAcceptances{}
	}
	if deps.Runs == nil {
		deps.Runs = &fakeRunProjection{}
	}
	if deps.Items == nil {
		deps.Items = &fakeItemProjection{}
	}
	if deps.Admissions == nil {
		deps.Admissions = testsupport.NewAdmissionGate()
	}
	if deps.Segments == nil {
		if seg.Observations == nil {
			seg.Observations = observer
		}
		if seg.Releases == nil {
			seg.Releases = deps.Releases
		}
		if seg.Publication.Openings == nil {
			seg.Publication.Openings = effects
		}
		if seg.Publication.Events == nil {
			seg.Publication.Events = effects
		}
		if seg.Publication.Barriers == nil {
			seg.Publication.Barriers = effects
		}
		if seg.Publication.Workspace == nil {
			seg.Publication.Workspace = effects
		}
		if seg.ChildStarts == nil {
			seg.ChildStarts = effects
		}
		if seg.Finalizer == nil {
			seg.Finalizer = effects
		}
		if seg.Runs == nil {
			seg.Runs = deps.Runs
		}
		if seg.Admissions == nil {
			seg.Admissions = deps.Admissions
		}
		if seg.Now == nil {
			seg.Now = func() time.Time { return time.Unix(1, 0).UTC() }
		}
		if seg.Retention == (Retention{}) {
			seg.Retention = DefaultRetention()
		}
		if seg.NewRunID == nil {
			seg.NewRunID = func() string { return "run_test" }
		}
		if seg.NewSegmentID == nil {
			seg.NewSegmentID = func() string { return "segment_test" }
		}
		segments, err := NewSegments(seg)
		if err != nil {
			panic(err)
		}
		deps.Segments = segments
	}
	return deps
}

type noCommandAcceptances struct{}

func (noCommandAcceptances) Lookup(context.Context, string) (CommandAcceptanceRecord, error) {
	return CommandAcceptanceRecord{}, nil
}
