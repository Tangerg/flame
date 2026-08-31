// Package maintenance implements safe-boundary model-context compaction plus
// the post-Run long-term memory and governed Skill-learning pipeline.
//
// These workers call the utility model directly, bypassing chat history, tools,
// and guardrails so their own prompts never enter the interactive conversation.
// A model-call compaction result is atomically installed in durable history and
// Interaction recovery state before the main model sees it. The workers share
// only this package's transcript rendering; the generic middleware-free call
// belongs to adapter/model.
//
// Pipeline owns ordering and failure aggregation for a clean Run boundary. The
// execution adapter supplies finished-Run facts and observes the result.
package maintenance
