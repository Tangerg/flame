package terminal

import (
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// sessionState owns the active Runtime projection and the lease that prevents
// work started for an older Session from mutating its replacement.
type sessionState struct {
	current         agent.Session
	context         *sessionContextLease
	invalidated     bool
	draftTransition *sessionDraftTransition
}

// executionState owns the one live root execution observed by the terminal.
// Durable Run facts remain owned by Runtime; these fields only track the local
// stream, cancellation, and elapsed-time presentation lifecycle.
type executionState struct {
	conversation     *agent.Conversation
	openingRunID     string
	pendingCancel    *pendingCancellation
	following        bool
	projectionFailed bool
	stopClock        func()
	clock            activeDurationClock
}

func (e executionState) observing() bool {
	return e.conversation.Busy() || e.following
}

func (e executionState) blocksAdmission() bool {
	return e.observing() || e.pendingCancel != nil
}

func (a *app) runAdmissionBlocked() bool {
	return a.execution.blocksAdmission() || a.session.invalidated || a.operations.BlocksRunAdmission()
}

func (a *app) runtimeChangeBlocksRunAdmission() bool {
	return a.session.invalidated || a.operations.BlocksRunAdmission()
}

// dialogState owns transient modal presentation. Keeping it separate from the
// application root makes it impossible to mistake dialog drafts, selections,
// or readers for durable Runtime state.
type dialogState struct {
	approval            *agent.Approval
	approvalDraft       *approvalDecisionDraft
	approvalArguments   string
	approvalOverride    *agent.ToolArgumentOverride
	approvalSections    []ToolSection
	approvalEditor      *contextEditorSession
	approvalForm        *headless.Form
	approvalPane        approvalPane
	approvalDialog      *presentationDialog
	sessionCenter       *sessionCenterPane
	sessionDialog       *presentationDialog
	sessionRenameDialog *kit.Dialog
	sessionDeleteDialog *kit.Dialog
	confirmationDialog  *kit.Dialog
	workspacePicker     *picker[workspaceChoice]
	workspaceDialog     *presentationDialog
	timeline            *timelinePane
	timelineDialog      *presentationDialog
	modelPicker         *picker[protocol.Model]
	modelDialog         *presentationDialog
	approvalModePicker  *picker[protocol.ApprovalMode]
	approvalModeDialog  *presentationDialog
	providerDialog      *kit.Dialog
	mcpDialog           *kit.Dialog
	scheduleDialog      *kit.Dialog
	activeContextEditor *contextEditorSession
	questionnaire       *questionnaire
	questionDialog      *kit.Dialog
	interactionReview   *interactionReview
	reviewDialog        *kit.Dialog
	commandPicker       *picker[commandPaletteItem]
	commandDialog       *presentationDialog
	shortcutDialog      *presentationDialog
	shortcutViewport    *headless.Viewport
	searchDialog        *presentationDialog
	reader              *readerPane
	readerDialog        *presentationDialog
	readerSearchDialog  *presentationDialog
	readerSearchQuery   string
	workspaceReader     workspaceReaderMode
	runtimeReader       runtimeReaderMode
	runtimeSelection    runtimeReaderSelection
	mcpToolServer       string
	mcpAuthorizationID  string
	queueDialog         *headless.Dialog
	searchQuery         string
}
