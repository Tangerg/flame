package models

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestSetUtilityRoleReturnsChatModelValidatorError(t *testing.T) {
	fail := errors.New("build client")
	state := NewRoleState(modelref.Role{})
	cfg := configuredRoleConfig(t)
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = &fakeChatModelValidator{err: fail}
	c := newTestCoordinator(cfg)

	if _, err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); !errors.Is(err, fail) {
		t.Fatalf("SetUtilityRole err = %v, want %v", err, fail)
	}
}

func TestSetUtilityRoleRequiresAConfiguredProvider(t *testing.T) {
	state := NewRoleState(modelref.Role{})
	cfg := configuredRoleConfig(t)
	cfg.Providers = &testProviderRegistry{}
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = staticChatModelValidator{}
	c := newTestCoordinator(cfg)

	_, err := c.SetUtilityRole(t.Context(), "anthropic", "claude-haiku")
	if !errors.Is(err, ErrProviderUnconfigured) {
		t.Fatalf("SetUtilityRole error = %v, want ErrProviderUnconfigured", err)
	}
}

func TestCommittedRoleUpdatesPublishModelsInvalidation(t *testing.T) {
	var notices []invalidation.Notice
	cfg := configuredRoleConfig(t)
	cfg.UtilityRoleState = NewRoleState(modelref.Role{})
	cfg.UtilityValidator = staticChatModelValidator{}
	cfg.UtilityStore = &fakeUtilityRoleSaver{}
	cfg.EmbeddingRoleState = NewRoleState(modelref.Role{})
	cfg.EmbeddingValidator = staticEmbeddingResolver{}
	cfg.EmbeddingStore = &fakeEmbeddingRoleSaver{}
	cfg.Invalidations = func(notice invalidation.Notice) { notices = append(notices, notice) }
	c := newTestCoordinator(cfg)

	if _, err := c.SetUtilityRole(t.Context(), "anthropic", "claude-haiku"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetEmbeddingRole(t.Context(), "openai", "text-embedding-3-small"); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 2 {
		t.Fatalf("notices = %+v, want two", notices)
	}
	for _, notice := range notices {
		if notice.Resource != invalidation.Models {
			t.Fatalf("notice = %+v, want models", notice)
		}
	}
}

func TestSetEmbeddingRoleRejectsProviderWithoutEmbeddings(t *testing.T) {
	state := NewRoleState(modelref.Role{})
	cfg := configuredRoleConfig(t)
	cfg.Catalog = testCatalog{metadata: []ProviderMetadata{
		providerMetadataFixture(t, "anthropic", ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability()),
	}}
	cfg.EmbeddingRoleState = state
	cfg.EmbeddingValidator = staticEmbeddingResolver{}
	c := newTestCoordinator(cfg)

	_, err := c.SetEmbeddingRole(t.Context(), "anthropic", "embedding")
	if !errors.Is(err, ErrEmbeddingUnsupported) {
		t.Fatalf("SetEmbeddingRole error = %v, want ErrEmbeddingUnsupported", err)
	}
}

func TestSetUtilityRoleSerializesPersistAndPublish(t *testing.T) {
	state := NewRoleState(modelref.Role{})
	saver := newBlockingUtilitySaver()
	cfg := configuredRoleConfig(t)
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = staticChatModelValidator{}
	cfg.UtilityStore = saver
	c := newTestCoordinator(cfg)

	assertRoleMutationSerializesPersistAndPublish(t, state, saver.blockingRoleSaver, c.SetUtilityRole)
}

func TestSetEmbeddingRoleSerializesPersistAndPublish(t *testing.T) {
	state := NewRoleState(modelref.Role{})
	saver := newBlockingEmbeddingSaver()
	cfg := configuredRoleConfig(t)
	cfg.EmbeddingRoleState = state
	cfg.EmbeddingValidator = staticEmbeddingResolver{}
	cfg.EmbeddingStore = saver
	c := newTestCoordinator(cfg)

	assertRoleMutationSerializesPersistAndPublish(t, state, saver.blockingRoleSaver, c.SetEmbeddingRole)
}

type roleMutation func(context.Context, string, string) (modelref.Role, error)

func assertRoleMutationSerializesPersistAndPublish(t *testing.T, state *RoleState, saver *blockingRoleSaver, setRole roleMutation) {
	t.Helper()
	first := make(chan error, 1)
	go func() { _, err := setRole(t.Context(), "provider", "first"); first <- err }()
	<-saver.firstStarted
	second := make(chan error, 1)
	go func() { _, err := setRole(t.Context(), "provider", "second"); second <- err }()
	select {
	case <-saver.secondEntered:
		t.Fatal("second mutation entered persistence before the first published")
	case <-time.After(20 * time.Millisecond):
	}
	close(saver.releaseFirst)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if got := saver.savedModel(); got != "second" {
		t.Fatalf("persisted model = %q, want second", got)
	}
	if role := state.Role(); role.Model() != "second" {
		t.Fatalf("live role = %+v, want second", role)
	}
}

type fakeUtilityRoleSaver struct {
	provider string
	model    string
	calls    int
}

func (f *fakeUtilityRoleSaver) SaveUtilityRole(_ context.Context, role modelref.Role) error {
	f.calls++
	f.provider = role.Provider()
	f.model = role.Model()
	return nil
}

type fakeChatModelValidator struct {
	provider string
	model    string
	err      error
}

func (f *fakeChatModelValidator) ValidateChatModel(_ context.Context, provider, model string) error {
	f.provider = provider
	f.model = model
	return f.err
}

type fakeEmbeddingRoleSaver struct {
	provider string
	model    string
	calls    int
}

func (f *fakeEmbeddingRoleSaver) SaveEmbeddingRole(_ context.Context, role modelref.Role) error {
	f.calls++
	f.provider = role.Provider()
	f.model = role.Model()
	return nil
}

type staticChatModelValidator struct{}

func (staticChatModelValidator) ValidateChatModel(context.Context, string, string) error { return nil }

type staticEmbeddingResolver struct{}

func (staticEmbeddingResolver) ValidateEmbeddingModel(context.Context, string, string) error {
	return nil
}

func configuredRoleConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Providers: &testProviderRegistry{entries: map[string]provider.Provider{
			"anthropic": testsupport.MustProvider("anthropic", "key", ""),
			"openai":    testsupport.MustProvider("openai", "key", ""),
			"provider":  testsupport.MustProvider("provider", "key", ""),
		}},
		Catalog: testCatalog{metadata: []ProviderMetadata{
			providerMetadataFixture(t, "anthropic", ProviderEndpointOptional, ProviderModelsBundled, EmbeddingCapabilityWithoutDefault()),
			providerMetadataFixture(t, "openai", ProviderEndpointOptional, ProviderModelsBundled, EmbeddingCapabilityWithoutDefault()),
			providerMetadataFixture(t, "provider", ProviderEndpointOptional, ProviderModelsBundled, EmbeddingCapabilityWithoutDefault()),
		}},
	}
}

type blockingRoleSaver struct {
	firstStarted  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
	mu            sync.Mutex
	model         string
}

func newBlockingRoleSaver() *blockingRoleSaver {
	return &blockingRoleSaver{
		firstStarted:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
}

func (b *blockingRoleSaver) save(model string) {
	b.mu.Lock()
	b.model = model
	b.mu.Unlock()
	if model == "first" {
		close(b.firstStarted)
		<-b.releaseFirst
		return
	}
	close(b.secondEntered)
}

func (b *blockingRoleSaver) savedModel() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.model
}

type blockingUtilitySaver struct{ *blockingRoleSaver }

func newBlockingUtilitySaver() *blockingUtilitySaver {
	return &blockingUtilitySaver{blockingRoleSaver: newBlockingRoleSaver()}
}

func (b *blockingUtilitySaver) SaveUtilityRole(_ context.Context, role modelref.Role) error {
	b.save(role.Model())
	return nil
}

type blockingEmbeddingSaver struct{ *blockingRoleSaver }

func newBlockingEmbeddingSaver() *blockingEmbeddingSaver {
	return &blockingEmbeddingSaver{blockingRoleSaver: newBlockingRoleSaver()}
}

func (b *blockingEmbeddingSaver) SaveEmbeddingRole(_ context.Context, role modelref.Role) error {
	b.save(role.Model())
	return nil
}

func completeTestConfig(cfg Config) Config {
	if cfg.Providers == nil {
		cfg.Providers = &testProviderRegistry{}
	}
	if cfg.Catalog == nil {
		cfg.Catalog = testCatalog{}
	}
	if cfg.Prober == nil {
		cfg.Prober = &fakeProber{}
	}
	if cfg.Lister == nil {
		cfg.Lister = &fakeLister{}
	}
	if cfg.UtilityRoleState == nil {
		cfg.UtilityRoleState = NewRoleState(modelref.Role{})
	}
	if cfg.UtilityValidator == nil {
		cfg.UtilityValidator = staticChatModelValidator{}
	}
	if cfg.UtilityStore == nil {
		cfg.UtilityStore = &fakeUtilityRoleSaver{}
	}
	if cfg.EmbeddingRoleState == nil {
		cfg.EmbeddingRoleState = NewRoleState(modelref.Role{})
	}
	if cfg.EmbeddingValidator == nil {
		cfg.EmbeddingValidator = staticEmbeddingResolver{}
	}
	if cfg.EmbeddingStore == nil {
		cfg.EmbeddingStore = &fakeEmbeddingRoleSaver{}
	}
	return cfg
}

func newTestCoordinator(cfg Config) *Coordinator {
	c, err := New(completeTestConfig(cfg))
	if err != nil {
		panic(err)
	}
	return c
}
