// Package maintenance implements safe-boundary model-context compaction plus
// the long-term memory and governed Skill-learning pipeline after successful
// Run completion commits and before its Segment stream closes.
//
// These workers call the utility model directly, bypassing chat history, tools,
// and guardrails so their own prompts never enter the interactive conversation.
// A model-call compaction result is atomically installed in durable history and
// Interaction recovery state before the main model sees it. The workers share
// only this package's transcript rendering; the generic middleware-free call
// belongs to adapter/integration/model.
//
// Pipeline owns ordering and failure aggregation. The execution adapter supplies
// the completed Run identity and joins the work before releasing admission.
package maintenance
