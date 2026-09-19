package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	KnowledgeList   Name = "knowledge.list"
	KnowledgeGet    Name = "knowledge.get"
	KnowledgeUpdate Name = "knowledge.update"
)

func registerKnowledge(registry *Registry) {
	registry.query(MethodMeta{
		Name: KnowledgeList, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, (*Handler).ListKnowledge)

	registry.query(MethodMeta{
		Name: KnowledgeGet, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, (*Handler).GetKnowledge)

	registry.command(MethodMeta{
		Name: KnowledgeUpdate, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
			protocol.ErrRevisionConflict.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, (*Handler).UpdateKnowledge)
}
