package client

import "github.com/Tangerg/flame/packages/go-sdk/protocol"

func pluginCallOptions(options protocol.PluginCallOptions) callConfig {
	return callConfig{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
	}
}

func callOptions(options protocol.CallOptions) callConfig {
	return callConfig{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func commandOptions(options protocol.CommandOptions) callConfig {
	return callConfig{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
	}
}

func runCommandOptions(options protocol.RunCommandOptions) callConfig {
	return callConfig{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
		AfterEventID:         options.AfterEventID,
	}
}

func runSubscriptionOptions(options protocol.RunSubscriptionOptions) callConfig {
	return callConfig{
		RequestMeta:  currentRequestMeta(options.RequestMeta),
		AfterEventID: options.AfterEventID,
	}
}

func subscriptionOptions(options protocol.SubscriptionOptions) callConfig {
	return callConfig{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func currentRequestMeta(meta protocol.RequestMeta) protocol.RequestMeta {
	if meta.ProtocolVersion == "" {
		meta.ProtocolVersion = protocol.Version
	}
	return meta
}

type callConfig struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}
