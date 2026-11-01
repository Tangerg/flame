package testsupport

import (
	"context"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/qimu/integrity"
)

type PlanAuthor struct{}

func (PlanAuthor) AuthorPlan(_ context.Context, source extension.ContributionSource, proposal planworkflow.Proposal) (planworkflow.AuthoredDocument, error) {
	if source == (extension.ContributionSource{}) {
		source = extension.ContributionSource{InstallationID: "eeb329cd-c7ce-40c9-bd90-6821fef06d30", Contribution: "test", Release: integrity.Hash([]byte("plan")).String()}
	}
	document, err := planworkflow.Seal(proposal)
	return planworkflow.AuthoredDocument{Document: document, Source: source}, err
}

func (PlanAuthor) ReviewPlan(_ context.Context, _ extension.ContributionSource, d planworkflow.Document) (string, error) {
	if len(d.Steps) == 0 {
		return "", nil
	}
	return d.Steps[0].Description, nil
}
