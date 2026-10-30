package main

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/httpresponse"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

const maxResponseBytes = 2 << 20

var errResponseLimit = errors.New("a2a: response exceeds the byte limit")

// Remote errors may embed credential-bearing URLs or arbitrary server bodies.
// Keep their causes for classification without publishing those strings.
type remoteError struct {
	operation string
	cause     error
}

func (e *remoteError) Error() string { return "a2a: " + e.operation + " failed" }
func (e *remoteError) Unwrap() error { return e.cause }

func (a agent) discover(ctx context.Context) (*a2aclient.Client, a2a.AgentInterface, error) {
	httpClient := httpresponse.NewClient(1<<20, errResponseLimit)
	httpClient.Timeout = 15 * time.Second
	httpClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		origin, err := httporigin.FromURL(request.URL)
		if err != nil || origin != a.cardOrigin || len(via) >= 10 {
			return errors.New("a2a: agent card redirect is outside its origin or limit")
		}
		return nil
	}
	card, err := agentcard.NewResolver(httpClient).Resolve(ctx, a.cardURL)
	if err != nil {
		return nil, a2a.AgentInterface{}, &remoteError{operation: "card discovery", cause: err}
	}
	var selected a2a.AgentInterface
	client, err := a2aclient.NewFromCard(ctx, card, a.clientOptions(&selected)...)
	if err != nil {
		return nil, a2a.AgentInterface{}, &remoteError{operation: "endpoint admission", cause: err}
	}
	return client, selected, nil
}

func (a agent) connect(ctx context.Context, endpoint a2a.AgentInterface) (*a2aclient.Client, error) {
	client, err := a2aclient.NewFromEndpoints(ctx, []*a2a.AgentInterface{&endpoint}, a.clientOptions(nil)...)
	if err != nil {
		return nil, &remoteError{operation: "bound endpoint admission", cause: err}
	}
	return client, nil
}

func (a agent) clientOptions(selected *a2a.AgentInterface) []a2aclient.FactoryOption {
	client := httpresponse.NewClient(maxResponseBytes, errResponseLimit)
	client.Timeout = 30 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("a2a: RPC redirects cannot replace the selected task endpoint")
	}
	factory := a2aclient.TransportFactoryFn(func(_ context.Context, _ *a2a.AgentCard, endpoint *a2a.AgentInterface) (a2aclient.Transport, error) {
		u, origin, err := parseEndpoint(endpoint.URL)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(a.origins, origin) {
			return nil, errors.New("a2a: RPC endpoint origin is not allowed")
		}
		if selected != nil {
			*selected = *endpoint
		}
		if endpoint.ProtocolBinding == a2a.TransportProtocolJSONRPC {
			return a2aclient.NewJSONRPCTransport(endpoint.URL, client), nil
		}
		return a2aclient.NewRESTTransport(u, client), nil
	})
	return []a2aclient.FactoryOption{
		a2aclient.WithDefaultsDisabled(),
		a2aclient.WithTransport(a2a.TransportProtocolJSONRPC, factory),
		a2aclient.WithTransport(a2a.TransportProtocolHTTPJSON, factory),
	}
}
