package runtime

import (
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func pluginCallOptions(options protocol.PluginCallOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
	}
}

func callOptions(options protocol.CallOptions) delivery.Options {
	return delivery.Options{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func commandOptions(options protocol.CommandOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
	}
}

func runCommandOptions(options protocol.RunCommandOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
		AfterEventID:         options.AfterEventID,
	}
}

func runSubscriptionOptions(options protocol.RunSubscriptionOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:  currentRequestMeta(options.RequestMeta),
		AfterEventID: options.AfterEventID,
	}
}

func subscriptionOptions(options protocol.SubscriptionOptions) delivery.Options {
	return delivery.Options{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func currentRequestMeta(meta protocol.RequestMeta) protocol.RequestMeta {
	if meta.ProtocolVersion == "" {
		meta.ProtocolVersion = protocol.Version
	}
	return meta
}
