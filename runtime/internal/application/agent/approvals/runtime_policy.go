package approvals

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// NewRuntimePolicy constructs permission policy over durable session modes and
// remembered rules. Only a Session may enter Plan mode; the Runtime default
// must remain one of the ordinary permission modes.
func NewRuntimePolicy(
	mode approval.Mode,
	store RuleStore,
	modeStore ModeStore,
	invalidations invalidation.Publish,
) (*RuntimePolicy, error) {
	if !mode.ValidDefault() {
		return nil, fmt.Errorf("%w: %q", approval.ErrInvalidMode, mode)
	}
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{"rule store", store}, {"session mode store", modeStore},
	} {
		value := reflect.ValueOf(dependency.value)
		if !value.IsValid() || ((value.Kind() == reflect.Pointer || value.Kind() == reflect.Map || value.Kind() == reflect.Func) && value.IsNil()) {
			return nil, fmt.Errorf("approvals: %s is required", dependency.name)
		}
	}
	p := &RuntimePolicy{store: store, modeStore: modeStore, invalidations: invalidations}
	p.mode.Store(&defaultModeState{mode: mode})
	return p, nil
}

// ModeStore persists explicit per-session permission state. Missing means use
// the runtime default. Implementations must return found=false for a missing
// session row and validate ownership at their persistence boundary.
type ModeStore interface {
	LookupMode(ctx context.Context, sessionID string) (state approval.SessionMode, found bool, err error)
	PutMode(ctx context.Context, sessionID string, state approval.SessionMode) error
	DeleteSession(ctx context.Context, sessionID string) error
}

// RuntimePolicy combines two policy facts consumed together at the tool-call
// boundary: session-effective permission mode and remembered approval rules.
// The default mode is atomic; Plan-mode transitions are serialized because
// they are rare state changes whose read/replace pair must be one process fact.
type RuntimePolicy struct {
	mode          atomic.Pointer[defaultModeState]
	modeMu        sync.Mutex
	modeStore     ModeStore
	store         RuleStore
	invalidations invalidation.Publish
}

// defaultModeState gives the atomically replaced default one immutable typed
// identity; it avoids translating the domain value through an implementation
// integer that could disagree with its durable and wire name.
type defaultModeState struct {
	mode approval.Mode
}

// DefaultMode returns the runtime fallback used by sessions without an explicit
// mode row.
func (r *RuntimePolicy) DefaultMode(_ context.Context) (approval.Mode, error) {
	state := r.mode.Load()
	if state == nil || !state.mode.ValidDefault() {
		return "", fmt.Errorf("%w: invalid stored default", approval.ErrInvalidMode)
	}
	return state.mode, nil
}

// SetDefaultMode changes the runtime fallback. Plan mode is session-only and is
// therefore rejected here.
func (r *RuntimePolicy) SetDefaultMode(_ context.Context, mode approval.Mode) error {
	if !mode.ValidDefault() {
		return fmt.Errorf("%w: %q", approval.ErrInvalidMode, mode)
	}
	r.mode.Store(&defaultModeState{mode: mode})
	r.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}

// Mode returns the effective mode for sessionID. An empty id reads the runtime
// default; a session with no explicit row also inherits that default.
func (r *RuntimePolicy) Mode(ctx context.Context, sessionID string) (approval.Mode, error) {
	fallback, err := r.DefaultMode(ctx)
	if err != nil {
		return "", err
	}
	if sessionID == "" {
		return fallback, nil
	}
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return "", fmt.Errorf("%w: %v", approval.ErrInvalidSessionMode, err)
	}
	state, found, err := r.modeStore.LookupMode(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !found {
		return fallback, nil
	}
	if err := state.Validate(); err != nil {
		return "", err
	}
	return state.Mode, nil
}

// EnterPlanMode narrows one session to read-only and records the permission mode
// it must regain on exit. It returns changed=false when already active.
func (r *RuntimePolicy) EnterPlanMode(ctx context.Context, sessionID string) (changed bool, err error) {
	if _, parseErr := resourceid.ParseSession(sessionID); parseErr != nil {
		return false, fmt.Errorf("%w: %v", approval.ErrInvalidSessionMode, parseErr)
	}
	r.modeMu.Lock()
	defer r.modeMu.Unlock()

	mode, err := r.Mode(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if mode == approval.ModePlan {
		return false, nil
	}
	state := approval.SessionMode{Mode: approval.ModePlan, RestoreMode: mode}
	if err := r.modeStore.PutMode(ctx, sessionID, state); err != nil {
		return false, err
	}
	return true, nil
}

// ExitPlanMode restores the exact mode captured by EnterPlanMode. It returns
// changed=false when the session is not in Plan mode.
func (r *RuntimePolicy) ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error) {
	if _, parseErr := resourceid.ParseSession(sessionID); parseErr != nil {
		return "", false, fmt.Errorf("%w: %v", approval.ErrInvalidSessionMode, parseErr)
	}
	r.modeMu.Lock()
	defer r.modeMu.Unlock()

	state, found, err := r.modeStore.LookupMode(ctx, sessionID)
	if err != nil {
		return "", false, err
	}
	if !found || state.Mode != approval.ModePlan {
		mode, modeErr := r.Mode(ctx, sessionID)
		return mode, false, modeErr
	}
	if err := state.Validate(); err != nil {
		return "", false, err
	}
	restored = state.RestoreMode
	if err := r.modeStore.PutMode(ctx, sessionID, approval.SessionMode{Mode: restored}); err != nil {
		return "", false, err
	}
	return restored, true, nil
}

func (r *RuntimePolicy) Decide(ctx context.Context, q approval.Query) (approval.Decision, bool, error) {
	if err := q.Validate(); err != nil {
		return "", false, err
	}
	candidates, err := r.visibleRules(ctx, q.SessionID, q.ProjectDir)
	if err != nil {
		return "", false, err
	}
	d, ok, err := approval.Decide(candidates, q)
	if err != nil {
		return "", false, err
	}
	return d, ok, nil
}

func (r *RuntimePolicy) Remember(ctx context.Context, req approval.RememberRequest) error {
	rule, err := req.Rule()
	if err != nil {
		return err
	}
	if err := r.store.Put(ctx, rule); err != nil {
		return err
	}
	r.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}

func (r *RuntimePolicy) Rules(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	return r.visibleRules(ctx, sessionID, projectDir)
}

func (r *RuntimePolicy) visibleRules(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	rules, err := r.store.Visible(ctx, sessionID, projectDir, approval.MaximumVisibleRules+1)
	if err != nil {
		return nil, err
	}
	if err := approval.ValidateVisibleRules(rules, sessionID, projectDir); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *RuntimePolicy) Forget(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: id is required", approval.ErrInvalidRule)
	}
	if err := r.store.Delete(ctx, id); err != nil {
		return err
	}
	r.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}
