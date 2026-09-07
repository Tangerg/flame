package bootstrap

import (
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

type conversationEnvironment struct {
	store    runs.ConversationStore
	messages *runs.ConversationHistory
}

func buildConversationEnvironment(store runs.ConversationStore, compactions runs.ConversationCompactionStore) (conversationEnvironment, error) {
	messages, err := runs.NewConversationHistory(store, compactions)
	if err != nil {
		return conversationEnvironment{}, err
	}
	return conversationEnvironment{
		store:    store,
		messages: messages,
	}, nil
}
