package delivery

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/protocol"
)

// stubCatalog declares the model source of one provider.
type stubCatalog struct {
	meta models.ProviderMetadata
}

func (s stubCatalog) Supported() []models.ProviderMetadata { return []models.ProviderMetadata{s.meta} }
func (s stubCatalog) Metadata(id string) (models.ProviderMetadata, bool) {
	if s.meta.ID() == id {
		return s.meta, true
	}
	return models.ProviderMetadata{}, false
}
func (stubCatalog) Models(string) []models.Model { return nil }
func (stubCatalog) LookupModel(string, string) (models.Model, bool) {
	return models.Model{}, false
}

// stubLister records whether the probe ran and returns canned ids/err.
type stubLister struct {
	ids   []string
	err   error
	calls int
}

func (s *stubLister) ListModels(context.Context, provider.Provider) ([]string, error) {
	s.calls++
	return s.ids, s.err
}

type stubRegistry struct{}

func (stubRegistry) List(context.Context) ([]provider.Provider, error) { return nil, nil }
func (stubRegistry) Get(context.Context, string) (provider.Provider, bool, error) {
	return provider.Provider{}, false, nil
}
func (stubRegistry) Update(context.Context, string, provider.Patch) (provider.Provider, error) {
	return provider.Provider{}, nil
}

type failingModelRegistry struct{ err error }

func (f failingModelRegistry) List(context.Context) ([]provider.Provider, error) {
	return nil, f.err
}

func (f failingModelRegistry) Get(context.Context, string) (provider.Provider, bool, error) {
	return provider.Provider{}, false, f.err
}

func (f failingModelRegistry) Update(context.Context, string, provider.Patch) (provider.Provider, error) {
	return provider.Provider{}, f.err
}

func probeHandler(meta models.ProviderMetadata, lister models.ProviderModelLister) *Handler {
	return handlerWithModels(models.Config{Providers: stubRegistry{}, Catalog: stubCatalog{meta: meta}, Lister: lister})
}

func listTestProviderModels(t *testing.T, s *Handler) []protocol.Model {
	t.Helper()
	page, err := s.ListModels(t.Context(), protocol.ListModelsRequest{Provider: "testprov"})
	if err != nil {
		t.Fatalf("ListModels error = %v", err)
	}
	return page.Data
}

func TestListModelsProbesEndpointAuthoritativeProvider(t *testing.T) {
	lister := &stubLister{ids: []string{"m-alpha", "m-beta"}}
	got := listTestProviderModels(t, probeHandler(serverProviderMetadata("testprov", models.ProviderEndpointRequired, models.ProviderModelsEndpoint, models.NoEmbeddingCapability()), lister))
	if lister.calls != 1 {
		t.Fatalf("lister calls = %d, want 1", lister.calls)
	}
	if len(got) != 2 || got[0].ID != "m-alpha" || got[1].ID != "m-beta" {
		t.Fatalf("models = %+v, want the probed ids", got)
	}
	if got[0].Provider != "testprov" {
		t.Fatalf("probed model provider = %q, want testprov", got[0].Provider)
	}
}

func TestListModelsPreservesEmptyEndpoint(t *testing.T) {
	lister := &stubLister{ids: nil}
	got := listTestProviderModels(t, probeHandler(serverProviderMetadata("testprov", models.ProviderEndpointRequired, models.ProviderModelsEndpoint, models.NoEmbeddingCapability()), lister))
	if lister.calls != 1 {
		t.Fatalf("lister calls = %d, want 1 (probe attempted)", lister.calls)
	}
	if len(got) != 0 {
		t.Fatalf("models = %+v, want empty endpoint result", got)
	}
}

func TestListModelsPreservesEndpointFailure(t *testing.T) {
	cause := errors.New("unreachable")
	lister := &stubLister{err: cause}
	handler := probeHandler(serverProviderMetadata("testprov", models.ProviderEndpointRequired, models.ProviderModelsEndpoint, models.NoEmbeddingCapability()), lister)
	page, err := handler.ListModels(t.Context(), protocol.ListModelsRequest{Provider: "testprov"})
	if !errors.Is(err, cause) || page != nil || lister.calls != 1 {
		t.Fatalf("ListModels = (%+v, %v), calls=%d; want endpoint failure", page, err, lister.calls)
	}
}

func TestListModelsPreservesCallerCancellation(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			lister := &stubLister{err: fmt.Errorf("model endpoint: %w", cause)}
			handler := probeHandler(
				serverProviderMetadata("testprov", models.ProviderEndpointRequired, models.ProviderModelsEndpoint, models.NoEmbeddingCapability()),
				lister,
			)
			endpoint := mustNewEndpoint(t, handler, EndpointConfig{})
			page, err := endpoint.Call[protocol.ListModelsRequest, *protocol.Page[protocol.Model]](
				t.Context(), ModelsList, protocol.ListModelsRequest{Provider: "testprov"}, Options{},
			)
			if page != nil || !errors.Is(err, cause) || lister.calls != 1 {
				t.Fatalf("ListModels = (%+v, %v), calls=%d; want endpoint %v", page, err, lister.calls, cause)
			}
		})
	}
}

func TestListModelsPreservesProviderRegistryFailure(t *testing.T) {
	sentinel := errors.New("registry unavailable")
	meta := serverProviderMetadata(
		"testprov",
		models.ProviderEndpointRequired,
		models.ProviderModelsEndpoint,
		models.NoEmbeddingCapability(),
	)
	server := handlerWithModels(models.Config{
		Providers: failingModelRegistry{err: sentinel},
		Catalog:   stubCatalog{meta: meta},
		Lister:    &stubLister{ids: []string{"must-not-mask-registry-failure"}},
	})

	page, err := server.ListModels(t.Context(), protocol.ListModelsRequest{Provider: "testprov"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("ListModels error = %v, want registry failure", err)
	}
	if page != nil {
		t.Fatalf("ListModels page = %+v, want no successful fallback after registry failure", page)
	}
}

func TestListModelsSkipsProbeForCatalogProvider(t *testing.T) {
	lister := &stubLister{ids: []string{"should-not-appear"}}
	got := listTestProviderModels(t, probeHandler(serverProviderMetadata("testprov", models.ProviderEndpointOptional, models.ProviderModelsBundled, models.NoEmbeddingCapability()), lister))
	if lister.calls != 0 {
		t.Fatalf("lister calls = %d, want 0 (catalog provider must not be probed)", lister.calls)
	}
	if len(got) != 0 {
		t.Fatalf("models = %+v, want empty (static catalog)", got)
	}
}

func TestListModelsMapsUnsupportedProviderToInvalidParams(t *testing.T) {
	server := probeHandler(
		serverProviderMetadata("testprov", models.ProviderEndpointOptional, models.ProviderModelsBundled, models.NoEmbeddingCapability()),
		new(stubLister),
	)

	page, err := server.ListModels(t.Context(), protocol.ListModelsRequest{Provider: "missing"})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("ListModels error = %v, want invalid params", err)
	}
	if page != nil {
		t.Fatalf("ListModels page = %+v, want nil", page)
	}
}
