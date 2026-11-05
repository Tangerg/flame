package main

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

var errEmptySignal = errors.New("feedback requires a recognized rating or nonblank text")

func prepare(in extension.FeedbackDraft) (plugin.FeedbackSignal, error) {
	if err := protocol.ValidateWireTree(protocol.FeedbackRequest{Source: in.Source, SessionID: in.SessionID, RunID: in.RunID, ItemID: in.ItemID, Rating: protocol.FeedbackRating(in.Rating), Text: in.Text}); err != nil {
		return plugin.FeedbackSignal{}, errors.Join(errEmptySignal, err)
	}
	return plugin.FeedbackSignal{Rating: protocol.FeedbackRating(in.Rating), Text: in.Text}, nil
}

func newServer(db *sql.DB) (*sdk.Server, error) {
	contract, err := plugin.FeedbackContract()
	if err != nil {
		return nil, err
	}
	prepareMethod, err := plugin.PrepareFeedbackMethod()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.feedback", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	classify := func(err error) (string, bool) { return "invalid_feedback", errors.Is(err, errEmptySignal) }
	if err := errors.Join(pluginstorage.RegisterReceipts(backend, db), pluginstorage.RegisterCommand(backend, db, prepareMethod, classify, func(_ context.Context, _ *sdk.ServerSession, in extension.FeedbackDraft) (plugin.FeedbackSignal, error) {
		return prepare(in)
	})); err != nil {
		return nil, err
	}
	if err := backend.BindCapability("feedback", contract); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
