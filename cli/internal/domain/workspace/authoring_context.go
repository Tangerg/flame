package workspace

import (
	"errors"
	"fmt"
	"strings"
)

type AuthoringDocumentScope string

const (
	AuthoringDocumentWorkingDirectory AuthoringDocumentScope = "cwd"
	AuthoringDocumentProjectRoot      AuthoringDocumentScope = "projectRoot"
	AuthoringDocumentHome             AuthoringDocumentScope = "home"
)

func (d AuthoringDocumentScope) Validate() error {
	switch d {
	case AuthoringDocumentWorkingDirectory, AuthoringDocumentProjectRoot, AuthoringDocumentHome:
		return nil
	default:
		return fmt.Errorf("agent document scope %q is invalid", d)
	}
}

type AuthoringDocument struct {
	Path  string
	Title string
	Scope AuthoringDocumentScope
}

func (d AuthoringDocument) Validate() error {
	if strings.TrimSpace(d.Path) == "" {
		return errors.New("agent document path is empty")
	}
	return d.Scope.Validate()
}

type AuthoringRecipeScope string

const (
	ProjectAuthoringRecipe AuthoringRecipeScope = "project"
	GlobalAuthoringRecipe  AuthoringRecipeScope = "global"
)

func (r AuthoringRecipeScope) Validate() error {
	if r != ProjectAuthoringRecipe && r != GlobalAuthoringRecipe {
		return fmt.Errorf("recipe scope %q is invalid", r)
	}
	return nil
}

type AuthoringRecipe struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string
	Scope        AuthoringRecipeScope
	Source       string
}

func (r AuthoringRecipe) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("recipe name is empty")
	}
	if strings.TrimSpace(r.Body) == "" {
		return fmt.Errorf("recipe %s body is empty", r.Name)
	}
	if strings.TrimSpace(r.Source) == "" {
		return fmt.Errorf("recipe %s source is empty", r.Name)
	}
	return r.Scope.Validate()
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
