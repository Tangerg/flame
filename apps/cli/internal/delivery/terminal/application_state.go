package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
)

// sessionState owns the active Runtime projection and the lease that prevents
// work started for an older Session from mutating its replacement.
type sessionState struct {
	current         conversation.Session
	context         *sessionContextLease
	invalidated     bool
	draftTransition *sessionDraftTransition
}

type openingReceipt struct {
	RunID     string
	CommandID authoring.CommandID
}

// executionState owns the local observation lifetime. Opening retains the
// acknowledged command association until projection or local settlement takes
// over; Runtime owns the product lifecycle facts.
type executionState struct {
	conversation     *conversation.Conversation
	opening          *openingReceipt
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
	approval            *conversation.Approval
	approvalDraft       *approvalDecisionDraft
	approvalArguments   string
	approvalOverride    *conversation.ToolArgumentOverride
	approvalSections    []toolSection
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
	approvalModePicker  *picker[protocol.ApprovalModePolicy]
	approvalModeDialog  *presentationDialog
	providerDialog      *kit.Dialog
	mcpForm             *mcpFormFlow
	scheduleDialog      *kit.Dialog
	activeContextEditor *contextEditorSession
	questionnaire       *questionnaire
	questionDialog      *kit.Dialog
	interruptReview     *interruptReview
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
	mcpToolServer       *protocol.MCPServerID
	mcpAuthorizationID  string
	queueDialog         *headless.Dialog
	searchQuery         string
}
