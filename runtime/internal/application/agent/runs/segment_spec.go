package runs

import (
	"context"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	corechat "github.com/Tangerg/scope/core/chat"
)

// segmentSpec is the prepared input to the package's segment supervisor.
type segmentSpec struct {
	// RunID is stable across every resumed Segment of the same logical Run.
	RunID string
	// SegmentID changes for every start or resume and scopes replay.
	SegmentID string
	SessionID string
	// WorkspaceCWD is the Session's project directory, which is what the durable
	// Run record and every product capability derived from it address. An
	// isolated Run executes in a scratch copy instead; only the executor is given
	// that path, through RootExecutionStart.CWD.
	WorkspaceCWD string
	// Isolated says the Run's tools write into that scratch copy rather than
	// WorkspaceCWD. Anything this layer reports against the workspace has to
	// account for it: the project directory does not change while such a Run
	// executes.
	Isolated   bool
	ExecutorID string
	// Opening is either a fresh root's admission or a parked tree's resumption.
	Opening segmentOpening
	// CommandAcceptance binds an externally initiated command to this
	// admission; its run_acceptance acceptance commits in the opening transaction.
	CommandAcceptance *CommandIdentity
	// admission transfers the pre-commit reservation to the live Run only after
	// its opening write-set commits.
	admission *ownership.RunAdmission
	// Fresh and crash-recovered executors stay with the staging caller until the
	// Segment takes ownership. A waiting continuation already has an owner.
	staged *stagedExecutionHandoff
	// BeginExecution crosses the executor side-effect boundary after opening commits.
	BeginExecution func(context.Context) error
	// DetachActivation is true for root Start/Resume commands whose durable
	// opening is their acceptance point. Their executor activation remains owned
	// by the Run lifecycle task but cannot retain the already-committed command
	// settlement. Composite commands leave this false when BeginExecution also
	// applies a post-commit Application transformation they must validate inline.
	DetachActivation bool
}

// segmentOpening is sealed: a Segment either admits a new root or resumes a
// parked tree, and each carries only the facts its own opening owns.
type segmentOpening interface{ segmentOpening() }

// rootOpening holds the admission facts of a fresh root. A resumed Segment
// reads the same facts from the parked Runs it continues.
type rootOpening struct {
	ModelSelection     modelchoice.Selection
	GoalIncarnationID  string
	InitialSession     *session.Session
	SessionReplacement *session.Replacement
	Actor              *operation.Actor
	CreatedAt          time.Time
	OpeningUserText    string
	Input              []transcript.ContentBlock
	// ConversationInput is the exact composed User message admitted to the
	// executor. Input remains the independently projected transcript content.
	ConversationInput *corechat.Message
	// ModelOnlyInput keeps an application-authored control message in the
	// provider conversation without publishing it as a user-visible transcript
	// Item. Only an autonomous Goal root may set it.
	ModelOnlyInput bool
	Capabilities   run.Capabilities
}

// treeResumption continues a parked or crash-recovered tree.
type treeResumption struct {
	Continuation *treeContinuation
	// Input is the human-authored follow-up the resumed root receives.
	Input []transcript.ContentBlock
	// ResumeAcceptance is the answer write-set a continuation opening consumes in
	// the same transaction: the open interrupt set and its settled Item
	// replacements commit with the resumed tree.
	ResumeAcceptance *ResumeAcceptance
	// StageExecution acquires the accepted continuation's executor resources. It
	// runs inside the same activation gate as observation and BeginExecution.
	StageExecution func(context.Context) error
	// CommitOpening is set only when a larger application transaction owns the
	// opening, such as waiting-subtree cancellation.
	CommitOpening func(context.Context, OpeningCommit) error
	toolAttempts  []OpenToolInvocation
}

func (rootOpening) segmentOpening()    {}
func (treeResumption) segmentOpening() {}

// restoresSegments reports a crash-recovered tree whose root was still running.
func (r treeResumption) restoresSegments(rootRunID string) bool {
	root, found := r.Continuation.run(rootRunID)
	return found && root.State() == run.Running
}

func (s segmentSpec) restoresSegments() bool {
	resumption, ok := s.Opening.(treeResumption)
	return ok && resumption.restoresSegments(s.RunID)
}

// stageExecution is the deferred executor staging of an accepted continuation.
func (s segmentSpec) stageExecution() (func(context.Context) error, bool) {
	resumption, ok := s.Opening.(treeResumption)
	if !ok || resumption.StageExecution == nil {
		return nil, false
	}
	return resumption.StageExecution, true
}

// openingUserText is the immutable fresh-root text a Session title derives from.
func (s segmentSpec) openingUserText() string {
	root, _ := s.Opening.(rootOpening)
	return root.OpeningUserText
}

func (s segmentSpec) executorRef() ExecutorRef {
	return ExecutorRef{SessionID: s.SessionID, ExecutorID: s.ExecutorID}
}
