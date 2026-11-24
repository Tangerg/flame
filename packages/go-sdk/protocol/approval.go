package protocol

type ApprovalRule struct {
	ModelName string               `json:"modelName"`
	Stale     bool                 `json:"stale"`
	ID        string               `json:"id"`
	Scope     ApprovalRuleScope    `json:"scope"`
	Tool      ToolRef              `json:"tool"`
	Subject   ApprovalSubject      `json:"subject"`
	Dir       string               `json:"dir,omitempty"`
	Decision  ApprovalRuleDecision `json:"decision"`
}

//flame:enum
type ApprovalRuleScope string

const (
	ApprovalRuleScopeSession ApprovalRuleScope = "session"
	ApprovalRuleScopeWorkspace ApprovalRuleScope = "workspace"
	ApprovalRuleScopeGlobal  ApprovalRuleScope = "global"
)

//flame:enum
type ApprovalRuleDecision string

const (
	ApprovalRuleDecisionAllow ApprovalRuleDecision = "allow"
	ApprovalRuleDecisionDeny  ApprovalRuleDecision = "deny"
)

type ListApprovalRulesRequest struct {
	SessionID string `json:"sessionId,omitempty"`
}

type ListApprovalRulesResult struct {
	Rules []ApprovalRule `json:"rules"`
}

type ForgetApprovalRuleRequest struct {
	ID string `json:"id"`
}

//flame:enum
type ApprovalMode string

const (
	ApprovalModeSafe     ApprovalMode = "safe"
	ApprovalModeBalanced ApprovalMode = "balanced"
	ApprovalModeYolo     ApprovalMode = "yolo"
)

type SetApprovalModeRequest struct {
	Mode ApprovalMode `json:"mode"`
}

//flame:enum
type ApprovalGate string

const (
	ApprovalGatePass   ApprovalGate = "pass"
	ApprovalGatePrompt ApprovalGate = "prompt"
	ApprovalGateDeny   ApprovalGate = "deny"
)

type ApprovalModePolicy struct {
	Mode    ApprovalMode `json:"mode"`
	Write   ApprovalGate `json:"write"`
	Exec    ApprovalGate `json:"exec"`
	Network ApprovalGate `json:"network"`
}

type ApprovalModeResult struct {
	Mode  ApprovalMode         `json:"mode"`
	Modes []ApprovalModePolicy `json:"modes"`
}

type SetApprovalRuleRequest struct {
	SessionID string               `json:"sessionId,omitempty"`
	Scope     ApprovalRuleScope    `json:"scope"`
	Tool      ToolRef              `json:"tool"`
	Subject   ApprovalSubject      `json:"subject"`
	Decision  ApprovalRuleDecision `json:"decision"`
}

//flame:enum
type ToolRefType string

const (
	ToolRefBuiltIn ToolRefType = "builtIn"
	ToolRefMCP     ToolRefType = "mcp"
)

type ToolRef struct {
	Type   ToolRefType  `json:"type"`
	Name   string       `json:"name,omitempty"`
	Server *MCPServerID `json:"server,omitzero"`
}

type ApprovalSubject struct {
	Type  ApprovalSubjectType `json:"type"`
	Value string              `json:"value,omitempty"`
}

//flame:enum
type ApprovalSubjectType string

const (
	ApprovalSubjectAll   ApprovalSubjectType = "all"
	ApprovalSubjectExact ApprovalSubjectType = "exact"
	ApprovalSubjectGlob  ApprovalSubjectType = "glob"
)
