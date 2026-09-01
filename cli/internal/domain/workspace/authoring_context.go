package workspace

import (
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

type AuthoringDocument struct {
	Path  string
	Title string
	Scope protocol.AgentDocScope
}

func (d AuthoringDocument) Validate() error {
	return (protocol.AgentDoc{Path: d.Path, Title: d.Title, Scope: d.Scope}).ValidateWire()
}

type AuthoringRecipe struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string
	Scope        protocol.RecipeScope
	Source       string
}

func (r AuthoringRecipe) Validate() error {
	return (protocol.Recipe{
		Name: r.Name, Description: r.Description, ArgumentHint: r.ArgumentHint,
		Body: r.Body, Scope: r.Scope, Source: r.Source,
	}).ValidateWire()
}

// Expand applies the runtime's documented client-side recipe substitution.
// $ARGUMENTS receives the trimmed input and $1..$9 receive whitespace-delimited
// arguments. A token such as $10 stays literal.
func (r AuthoringRecipe) Expand(arguments string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(arguments)
	parts := strings.Fields(trimmed)
	return expandRecipeTemplate(r.Body, trimmed, parts), nil
}

func expandRecipeTemplate(template, allArguments string, positional []string) string {
	var expanded strings.Builder
	expanded.Grow(len(template))
	for offset := 0; offset < len(template); {
		if strings.HasPrefix(template[offset:], "$ARGUMENTS") {
			expanded.WriteString(allArguments)
			offset += len("$ARGUMENTS")
			continue
		}
		if template[offset] == '$' && offset+1 < len(template) && template[offset+1] >= '1' && template[offset+1] <= '9' &&
			(offset+2 == len(template) || template[offset+2] < '0' || template[offset+2] > '9') {
			index := int(template[offset+1] - '1')
			if index < len(positional) {
				expanded.WriteString(positional[index])
			}
			offset += 2
			continue
		}
		expanded.WriteByte(template[offset])
		offset++
	}
	return expanded.String()
}
