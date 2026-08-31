package bootstrap

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/runs"
)

type conversationEnvironment struct {
	store    runs.ConversationStore
	messages *runs.ConversationHistory
}

func buildConversationEnvironment(store runs.ConversationStore, compactions runs.ConversationCompactionStore) (conversationEnvironment, error) {
	if store == nil {
		return conversationEnvironment{}, errors.New("runtime: ConversationStore is required")
	}
	if compactions == nil {
		return conversationEnvironment{}, errors.New("runtime: ConversationCompactions is required")
	}
	return conversationEnvironment{
		store:    store,
		messages: runs.NewConversationHistory(store, compactions),
	}, nil
}
