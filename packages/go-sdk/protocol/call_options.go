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

// CallMetadata is the transport-neutral form of one call's options. Every
// binding delivers this shape, so the typed option sets project into it here
// and an absent protocol version is defaulted once.
type CallMetadata struct {
	RequestMeta          RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

func (o CallOptions) Metadata() CallMetadata {
	return CallMetadata{RequestMeta: o.RequestMeta.current()}
}

func (o CommandOptions) Metadata() CallMetadata {
	return CallMetadata{RequestMeta: o.RequestMeta.current(), IdempotencyKey: o.IdempotencyKey, IdempotencyNamespace: o.IdempotencyNamespace}
}

func (o PluginCallOptions) Metadata() CallMetadata {
	return CallMetadata{RequestMeta: o.RequestMeta.current(), IdempotencyKey: o.IdempotencyKey, IdempotencyNamespace: o.IdempotencyNamespace}
}

func (o RunCommandOptions) Metadata() CallMetadata {
	return CallMetadata{
		RequestMeta: o.RequestMeta.current(), IdempotencyKey: o.IdempotencyKey,
		IdempotencyNamespace: o.IdempotencyNamespace, AfterEventID: o.AfterEventID,
	}
}

func (o RunSubscriptionOptions) Metadata() CallMetadata {
	return CallMetadata{RequestMeta: o.RequestMeta.current(), AfterEventID: o.AfterEventID}
}

func (o SubscriptionOptions) Metadata() CallMetadata {
	return CallMetadata{RequestMeta: o.RequestMeta.current()}
}

func (m RequestMeta) current() RequestMeta {
	if m.ProtocolVersion == "" {
		m.ProtocolVersion = Version
	}
	return m
}
