package protocol

// ApprovalRule is a standing decision bound to one tool source, scope, and
// explicitly typed subject matcher.
type ApprovalRule struct {
	ModelName string               `json:"modelName"`
	Stale     bool                 `json:"stale"`
	ID        string               `json:"id"`
	Scope     ApprovalRuleScope    `json:"scope"`
	Tool      ToolRef              `json:"tool"`
	Subject   ApprovalSubject      `json:"subject"`
	Dir       string               `json:"dir,omitempty"` // project-scope directory (display only; omitted otherwise)
	Decision  ApprovalRuleDecision `json:"decision"`
}

// ApprovalRuleScope is how far a remembered tool decision reaches.
type ApprovalRuleScope string

const (
	ApprovalRuleScopeSession ApprovalRuleScope = "session"
	ApprovalRuleScopeProject ApprovalRuleScope = "project"
	ApprovalRuleScopeGlobal  ApprovalRuleScope = "global"
)

// ApprovalRuleDecision is the only persisted rule verdict. It is distinct from
// an interrupt-response decision: one is a durable policy record, the other is
// a reply to a single pending approval.
type ApprovalRuleDecision string

const (
	ApprovalRuleDecisionAllow ApprovalRuleDecision = "allow"
	ApprovalRuleDecisionDeny  ApprovalRuleDecision = "deny"
)

// ListApprovalRulesRequest — approval.listRules body. SessionID anchors which
// session + project rules are visible (global rules always are).
type ListApprovalRulesRequest struct {
	SessionID string `json:"sessionId,omitempty"`
}

// ListApprovalRulesResult — the approval.listRules reply.
type ListApprovalRulesResult struct {
	Rules []ApprovalRule `json:"rules"`
}

// ForgetApprovalRuleRequest — approval.forgetRule body.
type ForgetApprovalRuleRequest struct {
	ID string `json:"id"`
}

// ApprovalMode is the runtime's default tool-permission stance
// (approval.getMode / approval.setMode). What each mode does with each tool
// class is published as ApprovalModeResult.modes, not restated by clients.
type ApprovalMode string

const (
	ApprovalModeSafe     ApprovalMode = "safe"
	ApprovalModeBalanced ApprovalMode = "balanced"
	ApprovalModeYolo     ApprovalMode = "yolo"
)

// SetApprovalModeRequest — approval.setMode body.
type SetApprovalModeRequest struct {
	Mode ApprovalMode `json:"mode"`
}

// ApprovalGate is what a mode does with a call of one tool safety class: run
// it, ask the user, or refuse it.
type ApprovalGate string

const (
	ApprovalGatePass   ApprovalGate = "pass"
	ApprovalGatePrompt ApprovalGate = "prompt"
	ApprovalGateDeny   ApprovalGate = "deny"
)

// ApprovalModePolicy is the gate one mode applies to each side-effecting tool
// class. Read-only (safe) tools never gate in any mode.
type ApprovalModePolicy struct {
	Mode    ApprovalMode `json:"mode"`
	Write   ApprovalGate `json:"write"`
	Exec    ApprovalGate `json:"exec"`
	Network ApprovalGate `json:"network"`
}

// ApprovalModeResult — the approval.getMode / setMode reply: the (new)
// current stance, and the policy of every mode a client may offer.
type ApprovalModeResult struct {
	Mode  ApprovalMode         `json:"mode"`
	Modes []ApprovalModePolicy `json:"modes"`
}

// SetApprovalRuleRequest records a standing decision against the current source
// authority. The server resolves the fingerprint; clients cannot supply one.
type SetApprovalRuleRequest struct {
	SessionID string               `json:"sessionId,omitempty"`
	Scope     ApprovalRuleScope    `json:"scope"`
	Tool      ToolRef              `json:"tool"`
	Subject   ApprovalSubject      `json:"subject"`
	Decision  ApprovalRuleDecision `json:"decision"`
}

type ToolRefType string

const (
	ToolRefBuiltIn ToolRefType = "builtIn"
	ToolRefMCP     ToolRefType = "mcp"
	ToolRefA2A     ToolRefType = "a2a"
)

// ToolRef is a source-qualified closed union. builtIn carries name; mcp
// carries server and the remote tool name; a2a carries endpoint. Name is exact
// source vocabulary; modelName belongs to the read projection, never the
// authority key.
type ToolRef struct {
	Type     ToolRefType  `json:"type"`
	Name     string       `json:"name,omitempty"`
	Endpoint string       `json:"endpoint,omitempty"`
	Server   *MCPServerID `json:"server,omitzero"`
}

// ApprovalSubject separates literal commands and paths from authored glob patterns.
type ApprovalSubject struct {
	Type  ApprovalSubjectType `json:"type"`
	Value string              `json:"value,omitempty"`
}

type ApprovalSubjectType string

const (
	ApprovalSubjectAll   ApprovalSubjectType = "all"
	ApprovalSubjectExact ApprovalSubjectType = "exact"
	ApprovalSubjectGlob  ApprovalSubjectType = "glob"
)
