package protocol

type CallOptions struct {
	RequestMeta RequestMeta
}

type CommandOptions struct {
	RequestMeta          RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
}

type PluginCallOptions struct {
	RequestMeta          RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
}

func (o PluginCallOptions) Clone() PluginCallOptions {
	o.RequestMeta = o.RequestMeta.Clone()
	return o
}

type RunCommandOptions struct {
	RequestMeta          RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

type RunSubscriptionOptions struct {
	RequestMeta  RequestMeta
	AfterEventID string
}

type SubscriptionOptions struct {
	RequestMeta RequestMeta
}

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
