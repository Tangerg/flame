package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/knowledge"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"

	framevalue "github.com/Tangerg/frame/value"
)

type embed func(context.Context, []string) (pluginsdk.TextEmbeddings, error)
type resolveEmbedding func(context.Context) (embed, error)

func remoteEmbedding(ctx context.Context) (embed, error) {
	ref, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, texts []string) (pluginsdk.TextEmbeddings, error) {
		return pluginsdk.CallTextEmbedding(ctx, ref, pluginmcp.NewCommandID(), texts)
	}, nil
}

type library struct {
	store   *store
	resolve resolveEmbedding
}

func newLibrary(store *store, resolve resolveEmbedding) (*library, error) {
	if store == nil || !store.db.Valid() || resolve == nil {
		return nil, errors.New("knowledge base: store and embedding resolver are required")
	}
	return &library{store: store, resolve: resolve}, nil
}

func (l *library) importDocument(ctx context.Context, method, id string, admitted framevalue.JSON) (knowledge.Header, error) {
	return l.indexDocument(ctx, method, id, admitted, func(context.Context) (knowledge.Draft, int64, error) {
		var input wire.ImportInput
		if err := admitted.Decode(&input); err != nil {
			return knowledge.Draft{}, 0, err
		}
		draft, err := knowledge.NewDraft(input.Content)
		return draft, input.ExpectedRevision, err
	})
}

func (l *library) reindexDocument(ctx context.Context, method, id string, admitted framevalue.JSON) (knowledge.Header, error) {
	return l.indexDocument(ctx, method, id, admitted, func(ctx context.Context) (knowledge.Draft, int64, error) {
		var input wire.DocumentInput
		if err := admitted.Decode(&input); err != nil {
			return knowledge.Draft{}, 0, err
		}
		document, err := l.store.read(ctx, input.ID)
		if err != nil {
			return knowledge.Draft{}, 0, err
		}
		draft, err := document.PrepareReindex(input.ExpectedRevision)
		return draft, input.ExpectedRevision, err
	})
}

func (l *library) indexDocument(ctx context.Context, method, id string, admitted framevalue.JSON, prepare func(context.Context) (knowledge.Draft, int64, error)) (knowledge.Header, error) {
	var embedDocument embed
	prepared, original, err := pluginstorage.AdmitCommand(ctx, l.store.db, method, id, admitted, func(ctx context.Context) error {
		var err error
		embedDocument, err = l.resolve(ctx)
		return err
	})
	if err != nil {
		return knowledge.Header{}, err
	}
	if prepared == nil {
		return pluginmcp.DecodeReply[knowledge.Header](original)
	}
	if !prepared.NewlyAdmitted() {
		return knowledge.Header{}, pluginstorage.ErrCommandUnconfirmed
	}
	draft, expectedRevision, err := prepare(ctx)
	if err != nil {
		return pluginstorage.CompleteCommand(ctx, prepared, classify, func(context.Context) (pluginstorage.CommandPlan[knowledge.Header], error) {
			return pluginstorage.CommandPlan[knowledge.Header]{}, err
		})
	}
	if _, err := l.store.indexTarget(ctx, draft.Content().ID, expectedRevision); err != nil {
		return pluginstorage.CompleteCommand(ctx, prepared, classify, func(context.Context) (pluginstorage.CommandPlan[knowledge.Header], error) {
			return pluginstorage.CommandPlan[knowledge.Header]{}, err
		})
	}
	result, err := embedDocument(ctx, draft.Texts())
	if err != nil {
		return pluginstorage.CompleteCommand(ctx, prepared, classify, func(context.Context) (pluginstorage.CommandPlan[knowledge.Header], error) {
			return pluginstorage.CommandPlan[knowledge.Header]{}, err
		})
	}
	indexed, err := draft.Index(result.Space, result.Vectors)
	if err != nil {
		return pluginstorage.CompleteCommand(ctx, prepared, classify, func(context.Context) (pluginstorage.CommandPlan[knowledge.Header], error) {
			return pluginstorage.CommandPlan[knowledge.Header]{}, err
		})
	}
	return pluginstorage.CompleteCommand(ctx, prepared, classify, func(ctx context.Context) (pluginstorage.CommandPlan[knowledge.Header], error) {
		return l.store.prepareReplacement(ctx, draft.Content().ID, expectedRevision, indexed)
	})
}

func (l *library) read(ctx context.Context, input wire.ReadInput) (wire.DocumentRead, error) {
	if _, err := identity.ParseOptionalProject(input.Project); err != nil {
		return wire.DocumentRead{}, err
	}
	document, err := l.store.read(ctx, input.ID)
	if err != nil {
		return wire.DocumentRead{}, err
	}
	// Another Project's document is absent here, not forbidden, so its identity
	// reveals nothing to a Run outside that Project.
	if document.Header().Revision == 0 || !document.VisibleIn(input.Project) {
		return wire.DocumentRead{}, knowledge.ErrNotFound
	}
	return wire.DocumentRead{Document: document.Header(), Text: document.Content().Text}, nil
}

func (l *library) list(ctx context.Context, input wire.ListInput) ([]knowledge.Header, error) {
	documents, err := l.visible(ctx, input.Project)
	if err != nil {
		return nil, err
	}
	headers := make([]knowledge.Header, len(documents))
	for i, document := range documents {
		headers[i] = document.Header()
	}
	return headers, nil
}

func (l *library) visible(ctx context.Context, project string) ([]knowledge.Document, error) {
	if _, err := identity.ParseOptionalProject(project); err != nil {
		return nil, err
	}
	documents, err := l.store.documents(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(documents, func(document knowledge.Document) bool { return !document.VisibleIn(project) }), nil
}

func (l *library) search(ctx context.Context, input wire.SearchInput) ([]knowledge.Match, error) {
	if strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > knowledge.MaxQueryRunes || input.Limit < 1 || input.Limit > knowledge.MaxResults {
		return nil, errors.New("knowledge base: a bounded query and result limit are required")
	}
	documents, err := l.visible(ctx, input.Project)
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
