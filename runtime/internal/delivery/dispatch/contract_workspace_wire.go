package dispatch

import (
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func registerDiffUnions(s *Shapes) {
	s.union(UnionSpec{GoType: typeOf[protocol.Diff](), Variants: []VariantSpec{
		{Tag: string(protocol.DiffFormatRows), Required: []string{"baseline", "rows"}},
		{Tag: string(protocol.DiffFormatRaw), Required: []string{"baseline", "patch"}},
		{Tag: string(protocol.DiffFormatVisual), Required: []string{"baseline", "visual"}},
	}})
	s.union(UnionSpec{GoType: typeOf[protocol.DiffBaseline](), Variants: []VariantSpec{
		{Tag: "head", Required: []string{"commit"}},
		{Tag: "mergeBase", Required: []string{"commit"}},
		{Tag: "emptyTree"},
	}})
	s.union(UnionSpec{
		GoType: typeOf[protocol.DiffRow](),
		Variants: []VariantSpec{
			{Tag: string(protocol.DiffRowHunk), Required: []string{"text"}},
			// The line numbers are `omitempty` because one flat struct serves four
			// tags, so an added row must be able to drop leftLine. They are REQUIRED
			// here anyway: a context row carries both, and a unified diff numbers
			// lines from 1, so the zero omitempty drops never occurs.
			{Tag: string(protocol.DiffRowContext), Required: []string{"code", "leftLine", "rightLine"}, Optional: []string{"noNewline"}},
			{Tag: string(protocol.DiffRowAdded), Required: []string{"code", "rightLine"}, Optional: []string{"noNewline"}},
			{Tag: string(protocol.DiffRowDeleted), Required: []string{"code", "leftLine"}, Optional: []string{"noNewline"}},
		},
	})
}

func registerWorkspaceConditions(s *Shapes) {
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ReadFileRequest](),
		Rules: []ConditionalRule{{
			When:     whenPresent("endLine"),
			Required: []string{"startLine"},
		}},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.FileContent](),
		Rules: []ConditionalRule{{
			When:     whenPresent("startLine"),
			Required: []string{"endLine"},
		}, {
			When:     whenPresent("endLine"),
			Required: []string{"startLine"},
		}},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.GetDiffRequest](),
		Rules: []ConditionalRule{{
			When:      whenEquals("format", string(protocol.DiffFormatRaw)),
			Forbidden: []string{"limit"},
		}},
	})
	workspaceChangeRules := []ConditionalRule{{
		When:     whenEquals("status", string(protocol.FileStatusRenamed)),
		Required: []string{"previousPath"},
	}, {
		When:      whenEquals("status", string(protocol.FileStatusAdded)),
		Forbidden: []string{"previousPath"},
	}, {
		When:      whenEquals("status", string(protocol.FileStatusModified)),
		Forbidden: []string{"previousPath"},
	}, {
		When:      whenEquals("status", string(protocol.FileStatusDeleted)),
		Forbidden: []string{"previousPath"},
	}, {
		When:      whenEquals("status", string(protocol.FileStatusUntracked)),
		Forbidden: []string{"previousPath"},
	}, {
		When:      whenPresent("binary"),
		Forbidden: []string{"added", "removed"},
	}}
	for _, shape := range []reflect.Type{
		typeOf[protocol.WorkspaceFileChange](),
		typeOf[protocol.FileDiff](),
	} {
		s.constraint(ObjectConstraintSpec{GoType: shape, Rules: workspaceChangeRules})
	}
}

func registerWorkspaceValues(s *Shapes) {
	nonEmpty[protocol.WorkspaceRef](s, "path")
	nonEmpty[protocol.WorkspaceInfo](s, "projectRoot")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.WorkspaceSummary](),
		Constraints: []FieldConstraint{
			{Field: "name", Kind: ConstraintNonEmpty},
			{Field: "sessionCount", Kind: ConstraintNonNegative},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.FileContent](),
		Constraints: []FieldConstraint{
			{Field: "totalLines", Kind: ConstraintPositive},
			{Field: "startLine", Kind: ConstraintPositive},
			{Field: "endLine", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.FileEntry](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "name", Kind: ConstraintNonEmpty},
			{Field: "sizeBytes", Kind: ConstraintNonNegative},
		},
	})
	nonNegative[protocol.GrepResult](s, "total")
	nonNegative[protocol.GrepCoverage](s, "scannedFiles", "skippedFiles", "remainingFiles", "sourceBytes")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GrepMatch](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "lineNumber", Kind: ConstraintPositive},
		},
	})
	workspaceChangeConstraints := []FieldConstraint{
		{Field: "path", Kind: ConstraintNonEmpty},
		{Field: "added", Kind: ConstraintNonNegative},
		{Field: "removed", Kind: ConstraintNonNegative},
	}
	for _, shape := range []reflect.Type{
		typeOf[protocol.FileDiff](),
		typeOf[protocol.WorkspaceFileChange](),
	} {
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: workspaceChangeConstraints})
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.DiffBaseline](),
		Constraints: []FieldConstraint{{Field: "commit", Kind: ConstraintPattern, Value: `^([0-9a-f]{40}|[0-9a-f]{64})$`}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.DiffRow](),
		Constraints: []FieldConstraint{
			{Field: "leftLine", Kind: ConstraintPositive},
			{Field: "rightLine", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GetDiffRequest](),
		Constraints: []FieldConstraint{
			{Field: "limit", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.DiffBudget](),
		Constraints: []FieldConstraint{
			{Field: "files", Kind: ConstraintPositive},
			{Field: "rows", Kind: ConstraintPositive},
			{Field: "bytes", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ReadFileRequest](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "startLine", Kind: ConstraintPositive},
			{Field: "endLine", Kind: ConstraintPositive},
			{Field: "maxBytes", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GrepRequest](),
		Constraints: []FieldConstraint{
			{Field: "query", Kind: ConstraintNonEmpty},
			{Field: "limit", Kind: ConstraintPositive},
		},
	})
}

func registerAuthoringContextValues(s *Shapes) {
	const nonBlankText = `\S`
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.AgentDoc](),
		Constraints: []FieldConstraint{{Field: "path", Kind: ConstraintPattern, Value: nonBlankText}},
	})
}
