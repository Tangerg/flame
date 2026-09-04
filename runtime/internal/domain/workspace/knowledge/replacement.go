package knowledge

// Replacement is one admitted human-authored document replacement. It binds
// the semantic scope, exact content revision read by the caller, and complete
// replacement content so persistence cannot execute a partially validated CAS.
type Replacement struct {
	scope            Scope
	expectedRevision string
	content          string
}

// NewReplacement constructs one validated Knowledge document replacement.
func NewReplacement(scope Scope, expectedRevision, content string) (Replacement, error) {
	replacement := Replacement{
		scope: scope, expectedRevision: expectedRevision, content: content,
	}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// Scope returns the semantic Knowledge location being replaced.
func (r Replacement) Scope() Scope { return r.scope }

// ExpectedRevision returns the exact content revision read by the caller.
func (r Replacement) ExpectedRevision() string { return r.expectedRevision }

// Content returns the complete replacement document.
func (r Replacement) Content() string { return r.content }

// Validate protects the complete CAS command at persistence boundaries.
func (r Replacement) Validate() error {
	if err := r.scope.Validate(); err != nil {
		return err
	}
	if r.expectedRevision == "" {
		return ErrRevisionRequired
	}
	return ValidateDocument(r.content)
}
