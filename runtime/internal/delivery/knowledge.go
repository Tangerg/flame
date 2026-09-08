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
	registry.Query(MethodMeta{
		Name: KnowledgeList, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, (*Handler).ListKnowledge)

	registry.Query(MethodMeta{
		Name: KnowledgeGet, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, (*Handler).GetKnowledge)

	registry.Command(MethodMeta{
		Name: KnowledgeUpdate, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
			protocol.ErrRevisionConflict.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, (*Handler).UpdateKnowledge)
}
