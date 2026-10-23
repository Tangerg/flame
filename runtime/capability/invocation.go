package capability

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Origin string

const (
	Human     Origin = "human"
	Model     Origin = "model"
	Automatic Origin = "automatic"
)

// Target is a concrete reference interpreted by the operation's resource owner.
// Scope is the method contract's target vocabulary, not a UI selection.
type Target struct {
	Scope     string `json:"scope"`
	Reference string `json:"reference"`
}

// Target scope vocabulary: the kind of resource an invocation targets. A method
// contract validates an invocation against the scope its operation owns.
const (
	ScopeCall         = "call"
	ScopeInstallation = "installation"
)

func (t Target) Validate() error {
	if !validToken(t.Scope) || t.Reference == "" || !utf8.ValidString(t.Reference) || strings.ContainsFunc(t.Reference, unicode.IsControl) {
		return errors.New("capability: concrete target scope and reference are required")
	}
	return nil
}

// Invocation is immutable calling-owner evidence. It contains no credentials,
// callback permissions, mutable execution state or business result.
type Invocation struct {
	ID     string `json:"id"`
	Origin Origin `json:"origin"`
	Parent string `json:"parent,omitzero"`
	Target Target `json:"target"`
}

func (i Invocation) Validate() error {
	if !validToken(i.ID) || i.Parent != "" && (!validToken(i.Parent) || i.Parent == i.ID) {
		return errors.New("capability: invocation and distinct parent identities are required")
	}
	if i.Origin != Human && i.Origin != Model && i.Origin != Automatic {
		return errors.New("capability: invocation origin must be human, model or automatic")
	}
	return i.Target.Validate()
}
