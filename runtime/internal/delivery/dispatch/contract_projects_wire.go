package dispatch

import "github.com/Tangerg/flame/packages/go-sdk/protocol"

func registerProjectUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType: typeOf[protocol.SessionProjectChange](),
		Variants: []VariantSpec{
			{Tag: string(protocol.SessionProjectSet), Required: []string{"projectId"}},
			{Tag: string(protocol.SessionProjectClear)},
		},
	})
}

func registerProjectValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Project](),
		Constraints: append(append(requiredResourceIdentity("id"),
			FieldConstraint{Field: "name", Kind: ConstraintPattern, Value: `\S`}), exactRevision()...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GetProjectRequest](), Constraints: requiredResourceIdentity("projectId"),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.CreateProjectRequest](),
		Constraints: []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: `\S`}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateProjectRequest](),
		Constraints: append(requiredResourceIdentity("projectId"),
			FieldConstraint{Field: "expectedRevision", Kind: ConstraintPositive},
			FieldConstraint{Field: "expectedRevision", Kind: ConstraintMaximum, Limit: protocol.MaximumExactJSONInteger},
			FieldConstraint{Field: "name", Kind: ConstraintNonEmpty},
		),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SessionProjectChange](), Constraints: resourceIdentity("projectId"),
	})
}
