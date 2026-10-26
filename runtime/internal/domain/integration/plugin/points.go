package plugin

import (
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/packages/go-sdk/point"
)

type Point struct {
	ID       ContributionID
	Contract jsoninput.Input
}

// PointValue is consumer-defined data, independent of executable exports.
// A value can name an operation; only that consumer interprets the reference.
type PointValue struct {
	ID       ContributionID
	Contract jsoninput.Input
	Value    jsoninput.Input
}

func (b *Builder) AdmitPoint(value Point) error {
	if err := value.ID.Validate(); err != nil {
		return err
	}
	contract, err := point.Parse(value.Contract.JSON())
	if err != nil {
		return refuse(DiagnosticInvalidDeclaration, "invalid point contract: %v", err)
	}
	if slices.ContainsFunc(b.declaration.Points, func(current Point) bool { return current.ID == value.ID || current.Contract == contract.Identity() }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate point %q", value.ID)
	}
	value.Contract = contract.Identity()
	b.declaration.Points = append(b.declaration.Points, value)
	return nil
}

func (b *Builder) AdmitPointValue(value PointValue) error {
	if err := value.ID.Validate(); err != nil {
		return err
	}
	contract, err := point.Parse(value.Contract.JSON())
	if err != nil {
		return refuse(DiagnosticInvalidDeclaration, "invalid point value contract: %v", err)
	}
	if err := contract.ValidateValue(value.Value); err != nil {
		return refuse(DiagnosticInvalidDeclaration, "point value %q: %v", value.ID, err)
	}
	if slices.ContainsFunc(b.declaration.PointValues, func(current PointValue) bool { return current.ID == value.ID }) || slices.ContainsFunc(b.declaration.Contributions, func(current Contribution) bool { return current.ID == value.ID }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate contribution %q", value.ID)
	}
	value.Contract = contract.Identity()
	b.declaration.PointValues = append(b.declaration.PointValues, value)
	return nil
}
