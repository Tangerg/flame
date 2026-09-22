package agentexec

// delegateDescription is the one model-facing meaning of delegate_task across
// executor strategies. Keeping it with delegateInput prevents strategies from teaching
// the model different semantics for the same Tool name.
const delegateDescription = "Delegate one self-contained task to a fresh Agent with coding tools and bounded delegation. " +
	"Use it for focused, separable work so the current context stays uncluttered. " +
	"The delegated Agent starts with clean context and cannot see its parent conversation, so include everything it needs in instructions. " +
	"It returns the agent's final response or direct tool results."

// delegateInput is the complete model-facing contract for one delegated task. Summary
// identifies the child in lifecycle projections; Instructions are the child's
// isolated input. Scope's generated schema is the sole admission contract;
// character limits and preserved instruction formatting must not be redefined
// by a second validator after the model has selected this Delegate.
type delegateInput struct {
	Summary      string `json:"summary" jsonschema:"minLength=1,maxLength=80,pattern=^[^[:space:]](.*[^[:space:]])?$" jsonschema_description:"Concise 3-5 word action label, at most 80 characters, that identifies this delegated task. Do not include leading or trailing whitespace."`
	Instructions string `json:"instructions" jsonschema:"minLength=1" jsonschema_description:"Complete self-contained work instructions. The delegated Agent cannot see the parent conversation, so include every fact it needs."`
}
