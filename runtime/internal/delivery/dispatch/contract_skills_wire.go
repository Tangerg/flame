package dispatch

import (
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
)

func registerSkillValues(s *Shapes) {
	const nonBlankText = `\S`
	skillName := []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: nonBlankText}}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillNameRequest](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillDetailRequest](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillDiagnostic](), Constraints: []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: nonBlankText}, {Field: "detail", Kind: ConstraintNonEmpty}}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillDetail](), Constraints: []FieldConstraint{
		{Field: "path", Kind: ConstraintNonEmpty}, {Field: "revision", Kind: ConstraintPattern, Value: `^[a-f0-9]{64}$`},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.Skill](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ManagedSkill](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SkillProposalRef](),
		Constraints: append(skillName,
			FieldConstraint{Field: "revision", Kind: ConstraintPattern, Value: domainskills.ProposalRevisionPattern},
		),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SkillProposal](),
		Constraints: []FieldConstraint{
			{Field: "name", Kind: ConstraintPattern, Value: nonBlankText},
			{Field: "revision", Kind: ConstraintPattern, Value: domainskills.ProposalRevisionPattern},
			{Field: "description", Kind: ConstraintPattern, Value: nonBlankText},
			{Field: "instructions", Kind: ConstraintPattern, Value: nonBlankText},
		},
	})
}
