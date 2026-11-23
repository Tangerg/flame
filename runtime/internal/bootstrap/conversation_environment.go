package bootstrap

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

type conversationEnvironment struct {
	store       *persistence.ConversationStore
	compactions runs.ConversationCompactionStore
	messages    *runs.ConversationHistory
}

func buildConversationEnvironment(
	store *persistence.ConversationStore,
	compactions runs.ConversationCompactionStore,
) (conversationEnvironment, error) {
	messages, err := runs.NewConversationHistory(store)
	if err != nil {
		return conversationEnvironment{}, fmt.Errorf("runtime: build conversation history: %w", err)
	}
	return conversationEnvironment{
		store:       store,
		compactions: compactions,
		messages:    messages,
	}, nil
}
