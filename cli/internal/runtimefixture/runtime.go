package runtimefixture

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

var errCanceled = errors.New("mock: run canceled")

const (
	defaultProvider = "mock"
	defaultModel    = "balanced"
)

type FaultKind string

const (
	FaultDisconnect FaultKind = "disconnect"
	FaultDuplicate  FaultKind = "duplicate"
	FaultConflict   FaultKind = "conflict"
)

type SubscriptionFault struct {
	Kind  FaultKind
	After int
}

// Runtime is an in-memory implementation of the CLI's consumer port. It models
// stable Runs, per-resume Segments, opaque event IDs, authoritative cold reads,
// and complete interrupt sets independently from any delivery transport.
type Runtime struct {
	// ToolModelNames supplies explicit catalog labels for approval command tests,
	// keyed by conversation.ToolSource because a reference holds a pointer.
	ToolModelNames map[string]string
	Instant        bool
	Script         func(authoredPrompt string) Script
	Faults         []SubscriptionFault

	mu           sync.Mutex
	sessions     map[string]*sessionState
	runs         map[string]*runState
	runOrder     []string
	rules        []storedRule
	approvalMode protocol.ApprovalMode
	fault        int
	identities   mockIdentitySequence
	now          func() time.Time
}

type sessionState struct {
	meta      conversation.Session
	items     []durableItem
	plan      *protocol.Plan
	planAtRun map[string]*protocol.Plan
	runs      []string
	active    string
}

type durableItem struct {
	runID string
	block conversation.Block
}

type storedRule struct {
	view      protocol.ApprovalRule
	sessionID string
}

type runState struct {
	id              string
	sessionID       string
	lineage         conversation.RunLineage
	provider        string
	model           string
	reasoningEffort string
	contextTokens   int64
	status          protocol.RunStatus
	active          string
	segments        map[string]*segmentState
	script          Script
	interrupts      []conversation.Interrupt
	answers         map[string]conversation.Answer
	cancel          chan struct{}
	cancelOnce      sync.Once
	usage           conversation.Usage
	outcome         conversation.Outcome
	// streaming holds the text previews Runtime would close as incomplete
	// Items before a terminal event; they are never durable while open.
	streaming []conversation.Block
}

type segmentState struct {
	id          string
	events      []conversation.RunEvent
	changed     chan struct{}
	closed      bool
	terminalErr error
}

func New() *Runtime {
	runtime := &Runtime{
		sessions:     make(map[string]*sessionState),
		runs:         make(map[string]*runState),
		approvalMode: protocol.ApprovalModeBalanced,
		now:          time.Now,
	}
	for _, session := range demoSessions() {
		runtime.sessions[session.ID] = &sessionState{meta: session}
	}
	runtime.seedHistory()
	return runtime
}

func (r *Runtime) ListModels(ctx context.Context) ([]protocol.Model, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	models := []protocol.Model{
		{ID: defaultModel, Provider: defaultProvider, DisplayName: "Mock Balanced", TokenLimits: mockModelTokenLimits(200_000, 32_000), Capabilities: &protocol.ModelCapabilities{Reasoning: true, ReasoningLevels: []string{"low", "medium", "high"}, ReasoningDefaultLevel: "medium", Multimodal: true, InputModalities: []protocol.Modality{protocol.ModalityText, protocol.ModalityImage}, OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true, StructuredOutput: true}},
		{ID: "fast", Provider: "mock", DisplayName: "Mock Fast", TokenLimits: mockModelTokenLimits(128_000, 16_000), Capabilities: &protocol.ModelCapabilities{InputModalities: []protocol.Modality{protocol.ModalityText}, OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true}},
		{ID: "deep", Provider: "synthetic", DisplayName: "Synthetic Deep", TokenLimits: mockModelTokenLimits(400_000, 64_000), Capabilities: &protocol.ModelCapabilities{Reasoning: true, ReasoningLevels: []string{"medium", "high", "max"}, ReasoningDefaultLevel: "high", InputModalities: []protocol.Modality{protocol.ModalityText}, OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true}},
	}
	return models, nil
}

func mockModelTokenLimits(contextWindow, maxOutput int64) *protocol.ModelTokenLimits {
	return &protocol.ModelTokenLimits{ContextWindow: &contextWindow, MaxOutputTokens: &maxOutput}
}

func (r *Runtime) GetApprovalMode(ctx context.Context) (protocol.ApprovalMode, error) {
	if err := context.Cause(ctx); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.approvalMode, nil
}

func (r *Runtime) SetApprovalMode(ctx context.Context, mode protocol.ApprovalMode) (protocol.ApprovalMode, error) {
	if err := context.Cause(ctx); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvalMode = mode
	return mode, nil
}
