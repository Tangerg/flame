// Package maintenance implements safe-boundary model-context compaction plus
// the long-term memory and governed Skill-learning pipeline after successful
// Scope Interaction completion, before committing the Flame Segment terminal fact.
//
// These workers call the utility model directly, bypassing chat history, tools,
// and guardrails so their own prompts never enter the interactive conversation.
// A model-call compaction result is atomically installed in durable history and
// Interaction recovery state before the main model sees it. The workers share
// only this package's transcript rendering; the generic middleware-free call
// belongs to adapter/integration/model.
//
// Pipeline owns ordering and failure aggregation while the Segment remains live.
// The execution adapter supplies completed Interaction evidence and joins the work.
package maintenance
