package main

import (
	"cmp"
	"slices"

	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

type reviewFindingSource struct {
	finding  wire.ReviewFinding
	revision wire.ReviewVersion
}

func (s reviewSnapshot) findingSource(revision, finding string) (reviewFindingSource, bool) {
	if revision != "" {
		if index := s.revision(revision); index >= 0 {
			r := s.Revisions[index]
			for _, item := range r.Findings {
				if item.ID == finding {
					return reviewFindingSource{finding: item, revision: r.ReviewVersion}, true
				}
			}
		}
	}
	return reviewFindingSource{}, false
}

func (s reviewSnapshot) recheckSources(previous string) ([]reviewFindingSource, bool) {
	if previous == "" {
		return nil, false
	}
	index := s.revision(previous)
	if index < 0 {
		return nil, true
	}
	prior := s.Revisions[index]
	sources := make([]reviewFindingSource, 0, len(prior.Findings))
	for _, finding := range prior.Findings {
		sources = append(sources, reviewFindingSource{finding: finding, revision: prior.ReviewVersion})
	}
	if prior.Recheck == nil {
		return sources, false
	}
	partial := prior.Recheck.FindingsPartial
	for _, check := range prior.Recheck.Findings {
		if decisions := check.Decisions; len(decisions) > 0 && decisions[len(decisions)-1].Disposition == wire.FindingResolved {
			continue
		}
		if len(sources) >= maxRevisionFindings {
			return sources, true
		}
		source, ok := s.findingSource(cmp.Or(check.SourceRevision, prior.Recheck.PreviousRevision), check.Finding)
		if !ok {
			partial = true
			continue
		}
		if !slices.ContainsFunc(sources, func(existing reviewFindingSource) bool { return existing.finding.ID == source.finding.ID }) {
			sources = append(sources, source)
		}
	}
	return sources, partial
}
