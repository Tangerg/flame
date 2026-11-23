package runtimefixture

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	balancedContextWindow   = 200_000
	balancedMaxOutputTokens = 32_000
	fastContextWindow       = 128_000
	fastMaxOutputTokens     = 16_000
	deepContextWindow       = 400_000
	deepMaxOutputTokens     = 64_000
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

type Runtime struct {
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
	openings     map[string]protocol.RunAcceptance
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
	streaming       []conversation.Block
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
		openings:     make(map[string]protocol.RunAcceptance),
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
		{ID: defaultModel, Provider: defaultProvider, DisplayName: "Mock Balanced", TokenLimits: mockModelTokenLimits(balancedContextWindow, balancedMaxOutputTokens), Capabilities: &protocol.ModelCapabilities{Reasoning: true, ReasoningLevels: []string{"low", "medium", "high"}, ReasoningDefaultLevel: "medium", Multimodal: true, InputModalities: []protocol.Modality{protocol.ModalityText, protocol.ModalityImage}, OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true, StructuredOutput: true}},
		{ID: "fast", Provider: defaultProvider, DisplayName: "Mock Fast", TokenLimits: mockModelTokenLimits(fastContextWindow, fastMaxOutputTokens), Capabilities: &protocol.ModelCapabilities{InputModalities: []protocol.Modality{protocol.ModalityText}, OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true}},
		{ID: "deep", Provider: "synthetic", DisplayName: "Synthetic Deep", TokenLimits: mockModelTokenLimits(deepContextWindow, deepMaxOutputTokens), Capabilities: &protocol.ModelCapabilities{Reasoning: true, ReasoningLevels: []string{"medium", "high", "max"}, ReasoningDefaultLevel: "high", InputModalities: []protocol.Modality{protocol.ModalityText}, OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true}},
	}
	return models, nil
}

func mockModelTokenLimits(contextWindow, maxOutput int64) *protocol.ModelTokenLimits {
	return &protocol.ModelTokenLimits{ContextWindow: &contextWindow, MaxOutputTokens: &maxOutput}
}

func approvalModePolicies() []protocol.ApprovalModePolicy {
	return []protocol.ApprovalModePolicy{
		{Mode: protocol.ApprovalModeSafe, Write: protocol.ApprovalGatePrompt, Exec: protocol.ApprovalGatePrompt, Network: protocol.ApprovalGatePrompt},
		{Mode: protocol.ApprovalModeBalanced, Write: protocol.ApprovalGatePass, Exec: protocol.ApprovalGatePrompt, Network: protocol.ApprovalGatePass},
		{Mode: protocol.ApprovalModeYolo, Write: protocol.ApprovalGatePass, Exec: protocol.ApprovalGatePass, Network: protocol.ApprovalGatePass},
	}
}

func (r *Runtime) GetApprovalMode(ctx context.Context) (protocol.ApprovalModeResult, error) {
	if err := context.Cause(ctx); err != nil {
		return protocol.ApprovalModeResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return protocol.ApprovalModeResult{Mode: r.approvalMode, Modes: approvalModePolicies()}, nil
}

func (r *Runtime) SetApprovalMode(ctx context.Context, mode protocol.ApprovalMode) (protocol.ApprovalModeResult, error) {
	if err := context.Cause(ctx); err != nil {
		return protocol.ApprovalModeResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvalMode = mode
	return protocol.ApprovalModeResult{Mode: mode, Modes: approvalModePolicies()}, nil
}
