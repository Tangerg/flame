package protocol

import "errors"

type ProblemData struct {
	Type                 string                  `json:"type"`
	Detail               string                  `json:"detail,omitempty"`
	DocURL               string                  `json:"docUrl,omitempty"`
	RetryAfterSeconds    int                     `json:"retryAfterSeconds,omitzero"`
	RequiredCapabilities []CapabilityRequirement `json:"requiredCapabilities,omitempty"`
	ActiveRun            *ActiveRunRef           `json:"activeRun,omitzero"`
	Errors               []FieldError            `json:"errors,omitempty"`
}

//flame:enum
type RecoveryAction string

const (
	RecoveryRefetch        RecoveryAction = "refetch"
	RecoveryColdRecover    RecoveryAction = "coldRecover"
	RecoveryResubscribe    RecoveryAction = "resubscribe"
	RecoveryReauthenticate RecoveryAction = "reauthenticate"
	RecoveryWaitRetryAfter RecoveryAction = "waitRetryAfter"
	RecoveryPromptUser     RecoveryAction = "promptUser"
	RecoveryStop           RecoveryAction = "stop"
)

func (r RecoveryAction) Valid() bool {
	switch r {
	case RecoveryRefetch,
		RecoveryColdRecover,
		RecoveryResubscribe,
		RecoveryReauthenticate,
		RecoveryWaitRetryAfter,
		RecoveryPromptUser,
		RecoveryStop:
		return true
	default:
		return false
	}
}

//flame:enum
type CapabilityRequirementType string

const (
	RequirementFeature       CapabilityRequirementType = "feature"
	RequirementInterruptType CapabilityRequirementType = "interruptType"
	RequirementRuntimeTopic  CapabilityRequirementType = "runtimeTopic"
)

type CapabilityRequirement struct {
	Type CapabilityRequirementType `json:"type"`
	Name string                    `json:"name"`
}

type ActiveRunRef struct {
	RunID  string    `json:"runId"`
	Status RunStatus `json:"status"`
}

type FieldError struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

const (
	ProblemInvalidRequest      = "invalid_request"
	ProblemInternalError       = "internal_error"
	ProblemAgentStuck          = "agent_stuck"
	ProblemRateLimited         = "rate_limited"
	ProblemInvalidCredentials  = "invalid_credentials"
	ProblemTimeout             = "timeout"
	ProblemProviderUnavailable = "provider_unavailable"
	ProblemProviderRejected    = "provider_rejected"
	ProblemDeniedByUser        = "denied_by_user"
	ProblemToolFailed          = "tool_failed"
	ProblemToolCanceled        = "tool_canceled"
	ProblemChildRunCanceled    = "child_run_canceled"
)

var (
	ErrInvalidRequest                  = errors.New(ProblemInvalidRequest)
	ErrInternalError                   = errors.New(ProblemInternalError)
	ErrMethodNotFound                  = errors.New("method_not_found")
	ErrInvalidParams                   = errors.New("invalid_params")
	ErrProviderError                   = errors.New("provider_error")
	ErrSessionNotFound                 = errors.New("session_not_found")
	ErrProjectNotFound                 = errors.New("project_not_found")
	ErrRunNotFound                     = errors.New("run_not_found")
	ErrItemNotFound                    = errors.New("item_not_found")
	ErrWorkspaceUnavailable            = errors.New("workspace_unavailable")
	ErrCapabilityNotNeg                = errors.New("capability_not_negotiated")
	ErrCheckpointUnavailable           = errors.New("checkpoint_unavailable")
	ErrCheckpointConflict              = errors.New("checkpoint_conflict")
	ErrPromptSourceTooLarge            = errors.New("prompt_source_too_large")
	ErrUnsupportedMime                 = errors.New("unsupported_mime")
	ErrPathOutsideRoot                 = errors.New("path_outside_root")
	ErrInterruptNotOpen                = errors.New("interrupt_not_open")
	ErrSessionHasActiveRun             = errors.New("session_has_active_run")
	ErrRunNotRoot                      = errors.New("run_not_root")
	ErrRunWaiting                      = errors.New("run_waiting")
	ErrRunFinished                     = errors.New("run_finished")
	ErrStaleSegment                    = errors.New("stale_segment")
	ErrReplayCursorInvalid             = errors.New("replay_cursor_invalid")
	ErrReplayUnavailable               = errors.New("replay_unavailable")
	ErrMCPServerNotFound               = errors.New("mcp_server_not_found")
	ErrSkillNotFound                   = errors.New("skill_not_found")
	ErrSkillUnavailable                = errors.New("skill_unavailable")
	ErrMCPServerAlreadyExists          = errors.New("mcp_server_already_exists")
	ErrMCPServerDisabled               = errors.New("mcp_server_disabled")
	ErrMCPAuthorizationAttemptNotFound = errors.New("mcp_authorization_attempt_not_found")
	ErrInvalidProtocolVersion          = errors.New("invalid_protocol_version")
	ErrVcsUnavailable                  = errors.New("vcs_unavailable")
	ErrSessionBusy                     = errors.New("session_busy")
	ErrRevisionConflict                = errors.New("revision_conflict")
	ErrIdempotencyConflict             = errors.New("idempotency_conflict")
	ErrIdempotencyInProgress           = errors.New("idempotency_in_progress")
	ErrIdempotencyStoreMismatch        = errors.New("idempotency_store_mismatch")
)

var (
	ErrPluginUnavailable      = errors.New("plugin_unavailable")
	ErrPluginNotFound         = errors.New("plugin_not_found")
	ErrPluginInUse            = errors.New("plugin_in_use")
	ErrPluginDisabled         = errors.New("plugin_disabled")
	ErrPluginStale            = errors.New("plugin_stale")
	ErrPluginCommandRetired   = errors.New("plugin_command_retired")
	ErrMCPOwnedByInstallation = errors.New("mcp_owned_by_installation")
	ErrPluginChanged          = errors.New("plugin_changed")
)
