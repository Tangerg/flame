package execution

import (
	"errors"
	"fmt"
	"strings"

	corechat "github.com/Tangerg/scope/core/chat"
	coremetadata "github.com/Tangerg/scope/core/metadata"
)

const contextProvenanceMetadataKey = "scope/context_provenance"

type contextSourceKind string

const (
	contextSourceBasePrompt     contextSourceKind = "base_prompt"
	contextSourcePinnedMemory   contextSourceKind = "pinned_memory"
	contextSourceAgentDocument  contextSourceKind = "agent_document"
	contextSourceSessionPlan    contextSourceKind = "session_plan"
	contextSourceLifecycleHook  contextSourceKind = "lifecycle_hook"
	contextSourceRecalledMemory contextSourceKind = "recalled_memory"
	contextSourceSessionGoal    contextSourceKind = "session_goal"
)

type contextPurpose string

const (
	contextPurposeInstruction contextPurpose = "instruction"
	contextPurposeData        contextPurpose = "data"
)

func (c contextSourceKind) source(reference string) contextSource {
	return contextSource{Kind: c, Reference: reference, Purpose: c.purpose()}
}

func (c contextSourceKind) purpose() contextPurpose {
	switch c {
	case contextSourcePinnedMemory,
		contextSourceRecalledMemory,
		contextSourceSessionGoal,
		contextSourceSessionPlan:
		return contextPurposeData
	case contextSourceBasePrompt,
		contextSourceAgentDocument,
		contextSourceLifecycleHook:
		return contextPurposeInstruction
	default:
		return ""
	}
}

type contextSource struct {
	Kind      contextSourceKind `json:"kind"`
	Reference string            `json:"reference,omitempty"`
	Purpose   contextPurpose    `json:"purpose"`
}

type contextSources []contextSource

func (c contextSources) validate() error {
	if len(c) == 0 {
		return errors.New("execution: empty context source set")
	}
	for index, source := range c {
		expectedPurpose := source.Kind.purpose()
		if expectedPurpose == "" || source.Purpose != expectedPurpose {
			return fmt.Errorf("execution: invalid context source[%d] kind %q purpose %q", index, source.Kind, source.Purpose)
		}
	}
	return nil
}

func (c contextSources) attach(target *coremetadata.Map, targetName string) error {
	if len(c) == 0 {
		return nil
	}
	if err := c.validate(); err != nil {
		return err
	}
	if err := target.Set(contextProvenanceMetadataKey, c); err != nil {
		return fmt.Errorf("execution: attach %s context provenance: %w", targetName, err)
	}
	return nil
}

type promptSection struct {
	text    string
	sources contextSources
}

type promptComposition struct {
	sections []promptSection
}

func (p *promptComposition) append(
	text string,
	source contextSource,
	additionalSources ...contextSource,
) {
	if text == "" {
		return
	}
	sources := make(contextSources, 1, 1+len(additionalSources))
	sources[0] = source
	sources = append(sources, additionalSources...)
	p.sections = append(p.sections, promptSection{
		text: text, sources: sources,
	})
}

func (p promptComposition) render() string {
	var rendered strings.Builder
	for index, section := range p.sections {
		if index > 0 {
			rendered.WriteString("\n\n")
		}
		rendered.WriteString(section.text)
	}
	return rendered.String()
}

func (p promptComposition) sources() contextSources {
	count := 0
	for _, section := range p.sections {
		count += len(section.sources)
	}
	sources := make(contextSources, 0, count)
	for _, section := range p.sections {
		sources = append(sources, section.sources...)
	}
	return sources
}

// runtimeContextMessage renders the composition as a User message framed as
// one kind of Runtime-authored context. Per-call state is sent after the
// conversation, where a System message would be hoisted ahead of it.
func (p promptComposition) runtimeContextMessage(kind RuntimeContextKind) (corechat.Message, error) {
	message := corechat.NewUserMessage(corechat.NewTextPart(FrameRuntimeContext(kind, p.render())))
	if err := p.sources().attach(&message.Metadata, string(kind)+" message"); err != nil {
		return corechat.Message{}, err
	}
	return message, nil
}

func (p promptComposition) systemMessage() (corechat.Message, error) {
	message := corechat.NewSystemMessage(p.render())
	if err := p.sources().attach(&message.Metadata, "system message"); err != nil {
		return corechat.Message{}, err
	}
	return message, nil
}
