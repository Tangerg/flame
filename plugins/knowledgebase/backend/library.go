package main

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/backend/domain"
	"github.com/Tangerg/flame/plugins/knowledgebase/backend/rpc"
	"github.com/google/uuid"
)

type embed func(context.Context, []string) (pluginsdk.TextEmbeddings, error)
type resolveEmbedding func(context.Context) (embed, error)

func remoteEmbedding(ctx context.Context) (embed, error) {
	ref, err := pluginsdk.CallbackFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, texts []string) (pluginsdk.TextEmbeddings, error) {
		return pluginsdk.CallTextEmbedding(ctx, ref, uuid.NewString(), texts)
	}, nil
}

type library struct {
	store   *store
	resolve resolveEmbedding
}

func newLibrary(store *store, resolve resolveEmbedding) (*library, error) {
	if store == nil || store.db == nil || resolve == nil {
		return nil, errors.New("knowledge base: store and embedding resolver are required")
	}
	return &library{store: store, resolve: resolve}, nil
}

func (l *library) importDocument(ctx context.Context, method, id string, input rpc.ImportInput, admitted jsoninput.Input) (knowledge.Header, error) {
	var embed embed
	prepared, original, err := pluginstorage.AdmitCommand(ctx, l.store.db, method, id, admitted, func(ctx context.Context) error {
		var err error
		embed, err = l.resolve(ctx)
		return err
	})
	if err != nil {
		return knowledge.Header{}, err
	}
	if prepared == nil {
		return pluginsdk.DecodeReply[knowledge.Header](original)
	}
	if !prepared.NewlyAdmitted() {
		return knowledge.Header{}, pluginstorage.ErrCommandUnconfirmed
	}
	draft, err := knowledge.NewDraft(input.Content)
	if err != nil {
		return l.store.completeImport(ctx, prepared, func(context.Context) (knowledge.Header, error) { return knowledge.Header{}, err })
	}
	result, err := embed(ctx, draft.Texts())
	if err != nil {
		return l.store.completeImport(ctx, prepared, func(context.Context) (knowledge.Header, error) { return knowledge.Header{}, err })
	}
	indexed, err := draft.Index(result.Space, result.Vectors)
	if err != nil {
		return l.store.completeImport(ctx, prepared, func(context.Context) (knowledge.Header, error) { return knowledge.Header{}, err })
	}
	return l.store.completeImport(ctx, prepared, func(ctx context.Context) (knowledge.Header, error) {
		return l.store.replace(ctx, input.Content.ID, input.ExpectedRevision, indexed)
	})
}

func (l *library) list(ctx context.Context) ([]knowledge.Header, error) {
	documents, err := l.store.documents(ctx)
	if err != nil {
		return nil, err
	}
	headers := make([]knowledge.Header, len(documents))
	for i, document := range documents {
		headers[i] = document.Header()
	}
	return headers, nil
}

func (l *library) search(ctx context.Context, input rpc.SearchInput) ([]knowledge.Match, error) {
	if strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > knowledge.MaxQueryRunes || input.Limit < 1 || input.Limit > knowledge.MaxResults {
		return nil, errors.New("knowledge base: a bounded query and result limit are required")
	}
	documents, err := l.store.documents(ctx)
	if err != nil {
		return nil, err
	}
	if len(documents) == 0 {
		return []knowledge.Match{}, nil
	}
	embed, err := l.resolve(ctx)
	if err != nil {
		return nil, err
	}
	result, err := embed(ctx, []string{input.Query})
	if err != nil {
		return nil, err
	}
	if len(result.Vectors) != 1 {
		return nil, knowledge.ErrInvalidIndex
	}
	matches := make([]knowledge.Match, 0)
	for _, document := range documents {
		found, err := document.Search(result.Space, result.Vectors[0])
		if err != nil {
			return nil, err
		}
		matches = append(matches, found...)
	}
	return knowledge.Rank(matches, input.Limit), nil
}

func classify(err error) string {
	if errors.Is(err, errStoredDocument) {
		return ""
	}
	return rpc.Classify(err)
}
