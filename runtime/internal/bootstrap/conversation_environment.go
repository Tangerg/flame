package bootstrap

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

type conversationEnvironment struct {
	store    runs.ConversationStore
	messages *runs.ConversationHistory
}

func buildConversationEnvironment(store runs.ConversationStore, compactions runs.ConversationCompactionStore) (conversationEnvironment, error) {
	history, err := runs.NewConversationHistory(store, compactions)
	if err != nil {
		return conversationEnvironment{}, fmt.Errorf("runtime: build conversation history: %w", err)
	}
	return conversationEnvironment{
		store:    store,
		messages: history,
	}, nil
}
