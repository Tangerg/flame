package runs

import (
	"context"
	"strings"
	"testing"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	corechat "github.com/Tangerg/scope/core/chat"
)

func TestNewCoordinatorRejectsMalformedDependencies(t *testing.T) {
	var typedNilStarter *fakeExecutionPorts
	tests := []struct {
		name string
		deps Dependencies
		want string
	}{
		{name: "empty", deps: Dependencies{}, want: "root execution starter"},
		{name: "typed nil", deps: Dependencies{RootStarts: typedNilStarter}, want: "root execution starter"},
		{name: "missing segments", deps: func() Dependencies {
			deps := completeTestDependencies(Dependencies{}, SegmentDependencies{})
			deps.Segments = nil
			return deps
		}(), want: "segment supervisor"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCoordinator(test.deps); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewCoordinator error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNewSegmentsRejectsMissingClockAndInvalidRetention(t *testing.T) {
	complete := func(change func(*SegmentDependencies)) SegmentDependencies {
		effects := &fakeEffects{}
		deps := SegmentDependencies{
			Observations: &fakeExecutor{}, Releases: &fakeExecutionPorts{},
			Publication: PublicationPorts{Openings: effects, Events: effects, Barriers: effects, Workspace: effects},
			ChildStarts: effects, Finalizer: effects, Runs: &fakeRunProjection{}, Admissions: testsupport.NewAdmissionGate(),
			Now: time.Now, Retention: DefaultRetention(),
			NewRunID: func() string { return "run" }, NewSegmentID: func() string { return "segment" },
		}
		change(&deps)
		return deps
	}
	if _, err := NewSegments(complete(func(*SegmentDependencies) {})); err != nil {
		t.Fatalf("NewSegments(complete) = %v", err)
	}
	for name, change := range map[string]func(*SegmentDependencies){
		"clock":     func(deps *SegmentDependencies) { deps.Now = nil },
		"retention": func(deps *SegmentDependencies) { deps.Retention = Retention{MaxEvents: -1, MaxBytes: 1} },
	} {
		if _, err := NewSegments(complete(change)); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("NewSegments without %s = %v", name, err)
		}
	}
}

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
func mustNewCoordinator(deps Dependencies) *testRuns {
	return mustNewCoordinatorWithSegments(deps, SegmentDependencies{}, cancellationOnly{})
}

// testRuns is the run use cases a test drives: the Coordinator and the
// Cancellations built over the same collaborators and Segments.
type testRuns struct {
	*Coordinator
	cancellations *Cancellations
}

func (t *testRuns) Cancel(ctx context.Context, cmd CancelCommand) (CancelResult, error) {
	return t.cancellations.Cancel(ctx, cmd)
}

// cancellationOnly are the collaborators only Cancellations uses; the shared
// ones come from the completed Dependencies.
type cancellationOnly struct {
	RootCancellation                   RunningRootCancellationRequester
	WaitingRestorer                    waitingExecutionRestorer
	RunningSubtreeCanceler             RunningSubtreeCanceler
	WaitingSubtreeCancellationPreparer WaitingSubtreeCancellationPreparer
	WaitingSubtreeCancellations        WaitingSubtreeCancellationCommitter
	Terminations                       terminationCommitter
}

// mustNewCoordinatorWithSegments also fixes the segment supervisor's and
// cancellation's collaborators; defaults are shared with the use cases' own.
func mustNewCoordinatorWithSegments(deps Dependencies, seg SegmentDependencies, only cancellationOnly) *testRuns {
	deps = completeTestDependencies(deps, seg)
	coordinator, err := NewCoordinator(deps)
	if err != nil {
		panic(err)
	}
	cancellations, err := NewCancellations(completeTestCancellations(deps, only))
	if err != nil {
		panic(err)
	}
	return &testRuns{Coordinator: coordinator, cancellations: cancellations}
}

func completeTestCancellations(deps Dependencies, only cancellationOnly) CancellationDependencies {
	control := &fakeExecutionPorts{}
	if only.RootCancellation == nil {
		only.RootCancellation = control
	}
	if only.WaitingRestorer == nil {
		only.WaitingRestorer = control
	}
	if only.RunningSubtreeCanceler == nil {
		only.RunningSubtreeCanceler = control
	}
	if only.WaitingSubtreeCancellationPreparer == nil {
		only.WaitingSubtreeCancellationPreparer = control
	}
	if only.WaitingSubtreeCancellations == nil {
		only.WaitingSubtreeCancellations = &fakeEffects{}
	}
	if only.Terminations == nil {
		// Session doubles implement every Session port; a test's own one also
		// records the terminations its cancellations commit.
		if terminations, ok := deps.Session.Interrupts.(terminationCommitter); ok {
			only.Terminations = terminations
		} else {
			only.Terminations = &fakeRunSessions{}
		}
	}
	return CancellationDependencies{
		RootCancellation:                   only.RootCancellation,
		WaitingRestorer:                    only.WaitingRestorer,
		RunningSubtreeCanceler:             only.RunningSubtreeCanceler,
		WaitingSubtreeCancellationPreparer: only.WaitingSubtreeCancellationPreparer,
		WaitingSubtreeCancellations:        only.WaitingSubtreeCancellations,
		Terminations:                       only.Terminations,
		Sessions:                           deps.Session.Reader,
		Interrupts:                         deps.Session.Interrupts,
		Checkpoints:                        deps.Checkpoints,
		Releases:                           deps.Releases,
		Segments:                           deps.Segments,
		Runs:                               deps.Runs,
		Items:                              deps.Items,
		Admissions:                         deps.Admissions,
	}
}

func completeTestDependencies(deps Dependencies, seg SegmentDependencies) Dependencies {
	control := &fakeExecutionPorts{}
	observer := &fakeExecutor{}
	sessions := &fakeRunSessions{}
	effects := &fakeEffects{}
	if deps.RootStarts == nil {
		deps.RootStarts = control
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
	if deps.Session.Reader == nil {
		deps.Session.Reader = sessions
	}
	if deps.Session.Creator == nil {
		deps.Session.Creator = sessions
	}
	if deps.Session.ActiveRuns == nil {
		deps.Session.ActiveRuns = sessions
	}
	if deps.Session.Interrupts == nil {
		deps.Session.Interrupts = sessions
	}
	if deps.Checkpoints == nil {
		deps.Checkpoints = effects
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
