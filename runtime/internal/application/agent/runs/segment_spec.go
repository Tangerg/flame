package runs

import (
	"context"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

type segmentSpec struct {
	RunID             identity.RunID
	SegmentID         identity.SegmentID
	SessionID         identity.SessionID
	WorkspaceCWD      string
	Isolated          bool
	ExecutorID        string
	Opening           segmentOpening
	CommandAcceptance *CommandIdentity
	admission         *ownership.RunAdmission
	staged            *stagedExecutionHandoff
	BeginExecution    func(context.Context) error
	DetachActivation  bool
}

type segmentOpening interface{ segmentOpening() }

type rootOpening struct {
	ModelSelection    flamemodel.Selection
	GoalIncarnationID goalref.IncarnationID
	SessionWrite      OpeningSessionWrite
	Actor             *operation.Actor
	CreatedAt         time.Time
	OpeningUserText   string
	Input             []transcript.ContentBlock
	ConversationInput *corechat.Message
	ModelOnlyInput    bool
	Capabilities      run.Capabilities
}

type treeResumption struct {
	Continuation     *treeContinuation
	Input            []transcript.ContentBlock
	ResumeAcceptance *ResumeAcceptance
	StageExecution   func(context.Context) error
	CommitOpening    func(context.Context, OpeningCommit) error
	toolAttempts     []OpenToolInvocation
}

func (o rootOpening) draft(spec segmentSpec) run.Draft {
	return run.Draft{
		RunID:             spec.RunID,
		SessionID:         spec.SessionID,
		SegmentID:         spec.SegmentID,
		ModelSelection:    o.ModelSelection,
		GoalIncarnationID: o.GoalIncarnationID,
		Capabilities:      o.Capabilities,
		CreatedAt:         o.CreatedAt,
	}
}

func (rootOpening) segmentOpening()    {}
func (treeResumption) segmentOpening() {}

func (r treeResumption) restoresSegments(rootRunID identity.RunID) bool {
	root, found := r.Continuation.run(rootRunID)
	return found && root.State() == run.Running
}

func (s segmentSpec) restoresSegments() bool {
	resumption, ok := s.Opening.(treeResumption)
	return ok && resumption.restoresSegments(s.RunID)
}

func (s segmentSpec) stageExecution() (func(context.Context) error, bool) {
	resumption, ok := s.Opening.(treeResumption)
	if !ok || resumption.StageExecution == nil {
		return nil, false
	}
	return resumption.StageExecution, true
}

func (s segmentSpec) openingUserText() string {
	root, _ := s.Opening.(rootOpening)
	return root.OpeningUserText
}

func (s segmentSpec) executorRef() ExecutorRef {
	return ExecutorRef{SessionID: s.SessionID, ExecutorID: s.ExecutorID}
}
