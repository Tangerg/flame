package runtime

import (
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// CallOptions describes one read or other non-mutating delivery.
type CallOptions struct {
	RequestMeta protocol.RequestMeta
}

// CommandOptions describes one mutation and its stable retry identity.
// SteerRun requires IdempotencyKey; other mutations permit omission.
type CommandOptions struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
}

// PluginCallOptions carries an optional command retry identity. Query methods
// reject it after the installed operation's contract is resolved.
type PluginCallOptions struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
}

func pluginCallOptions(options PluginCallOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
	}
}

// RunCommandOptions describes a Run-opening command. Retain IdempotencyKey,
// the discovered store namespace and the exact request before dispatch. StartRun
// and ResumeRun reject an absent key. AfterEventID applies only when redelivery
// reattaches to the original Segment.
type RunCommandOptions struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

// RunSubscriptionOptions identifies the last Run event already consumed.
type RunSubscriptionOptions struct {
	RequestMeta  protocol.RequestMeta
	AfterEventID string
}

// SubscriptionOptions describes a non-replayable Runtime-wide subscription.
type SubscriptionOptions struct {
	RequestMeta protocol.RequestMeta
}

func callOptions(options CallOptions) delivery.Options {
	return delivery.Options{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func commandOptions(options CommandOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
	}
}

func runCommandOptions(options RunCommandOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
		AfterEventID:         options.AfterEventID,
	}
}

func runSubscriptionOptions(options RunSubscriptionOptions) delivery.Options {
	return delivery.Options{
		RequestMeta:  currentRequestMeta(options.RequestMeta),
		AfterEventID: options.AfterEventID,
	}
}

func subscriptionOptions(options SubscriptionOptions) delivery.Options {
	return delivery.Options{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func currentRequestMeta(meta protocol.RequestMeta) protocol.RequestMeta {
	if meta.ProtocolVersion == "" {
		meta.ProtocolVersion = protocol.ProtocolVersion
	}
	return meta
}
