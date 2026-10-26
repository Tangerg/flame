package protocol

// CallOptions describes one read or other non-mutating delivery.
type CallOptions struct {
	RequestMeta RequestMeta
}

// CommandOptions describes one mutation and its stable retry identity.
// SteerRun requires IdempotencyKey; other mutations permit omission.
type CommandOptions struct {
	RequestMeta          RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
}

// PluginCallOptions carries an optional command retry identity. Query methods
// reject it after the installed operation's contract is resolved.
type PluginCallOptions struct {
	RequestMeta          RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
}

// RunCommandOptions describes a Run-opening command. Retain IdempotencyKey,
// the discovered store namespace and the exact request before dispatch. StartRun
// and ResumeRun reject an absent key. AfterEventID applies only when redelivery
// reattaches to the original Segment.
type RunCommandOptions struct {
	RequestMeta          RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

// RunSubscriptionOptions identifies the last Run event already consumed.
type RunSubscriptionOptions struct {
	RequestMeta  RequestMeta
	AfterEventID string
}

// SubscriptionOptions describes a non-replayable Runtime-wide subscription.
type SubscriptionOptions struct {
	RequestMeta RequestMeta
}
