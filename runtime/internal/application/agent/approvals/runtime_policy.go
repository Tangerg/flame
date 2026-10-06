package approvals

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// NewRuntimePolicy constructs permission policy over durable session modes and
// remembered rules. Only a Session may enter Plan mode; the Runtime default
// must remain one of the ordinary permission modes.
func NewRuntimePolicy(
	mode approval.Mode,
	store RuleStore,
	modeStore ModeStore,
	authorities SourceAuthorities,
	invalidations invalidation.Publish,
) (*RuntimePolicy, error) {
	if !mode.ValidDefault() {
		return nil, fmt.Errorf("%w: %q", approval.ErrInvalidMode, mode)
	}
	for _, required := range []struct {
		name  string
		value any
	}{
		{"source authorities", authorities}, {"rule store", store}, {"session mode store", modeStore},
	} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("approvals: %s is required", required.name)
		}
	}
	p := &RuntimePolicy{authorities: authorities, store: store, modeStore: modeStore, invalidations: invalidations}
	p.mode.Store(&defaultModeState{mode: mode})
	return p, nil
}

// ModeStore persists explicit per-session permission state. Missing means use
// the runtime default. Implementations must return found=false for a missing
// session row and validate ownership at their persistence boundary.
//
// Retiring a deleted Session's state is not here: the session write-set owns
// that, transactionally with everything else the delete removes, through its
// own cleaner port.
type ModeStore interface {
	PlanModeActive(ctx context.Context, sessionID string) (bool, error)
	StartPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
	EndPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
}

// RuntimePolicy combines two policy facts consumed together at the tool-call
// boundary: session-effective permission mode and remembered approval rules.
// The default mode is atomic; Plan-mode transitions are serialized because
// they are rare state changes whose read/replace pair must be one process fact.
type RuntimePolicy struct {
	mode          atomic.Pointer[defaultModeState]
	authorities   SourceAuthorities
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
// default; a Session outside Plan mode follows that default.
func (r *RuntimePolicy) Mode(ctx context.Context, sessionID string) (approval.Mode, error) {
	if sessionID != "" {
		if err := resourceid.ValidateSession(sessionID); err != nil {
			return "", fmt.Errorf("%w: %v", approval.ErrInvalidSessionMode, err)
		}
		planning, err := r.modeStore.PlanModeActive(ctx, sessionID)
		if err != nil {
			return "", err
		}
		if planning {
			return approval.ModePlan, nil
		}
	}
	return r.DefaultMode(ctx)
}

// EnterPlanMode narrows one session to read-only. It returns changed=false
// when already active.
func (r *RuntimePolicy) EnterPlanMode(ctx context.Context, sessionID string) (changed bool, err error) {
	if parseErr := resourceid.ValidateSession(sessionID); parseErr != nil {
		return false, fmt.Errorf("%w: %v", approval.ErrInvalidSessionMode, parseErr)
	}
	return r.modeStore.StartPlanMode(ctx, sessionID)
}

// ExitPlanMode returns the session to the Runtime default, reporting the mode
// it now follows. It returns changed=false when the session is not in Plan
// mode.
func (r *RuntimePolicy) ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error) {
	if parseErr := resourceid.ValidateSession(sessionID); parseErr != nil {
		return "", false, fmt.Errorf("%w: %v", approval.ErrInvalidSessionMode, parseErr)
	}
	changed, err = r.modeStore.EndPlanMode(ctx, sessionID)
	if err != nil {
		return "", false, err
	}
	restored, err = r.Mode(ctx, sessionID)
	return restored, changed, err
}

func (r *RuntimePolicy) Decide(ctx context.Context, q approval.Query) (approval.Decision, bool, error) {
	if err := q.Validate(); err != nil {
		return "", false, err
	}
	current, found, err := r.authorities.Fingerprint(ctx, q.Tool)
	if err != nil {
		return "", false, err
	}
	if !found || current != q.SourceFingerprint {
		return "", false, nil
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
	current, found, err := r.authorities.Fingerprint(ctx, req.Tool)
	if err != nil {
		return err
	}
	if !found || current != req.SourceFingerprint {
		return approval.ErrSourceAuthorityChanged
	}
	return r.put(ctx, rule)
}

func (r *RuntimePolicy) put(ctx context.Context, rule approval.Rule) error {
	if err := r.store.Put(ctx, rule); err != nil {
		return err
	}
	r.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}

type SourceAuthorities interface {
	Fingerprint(context.Context, tool.Ref) (fingerprint.Digest, bool, error)
	Fingerprints(context.Context, []tool.Ref) (map[tool.Ref]fingerprint.Digest, error)
}
type RuleView struct {
	ID         string
	Scope      approval.Scope
	ProjectDir string
	Tool       tool.Ref
	Subject    approval.Subject
	Decision   approval.Decision
	Stale      bool
}

func (r *RuntimePolicy) Rules(ctx context.Context, sessionID, projectDir string) ([]RuleView, error) {
	rules, err := r.visibleRules(ctx, sessionID, projectDir)
	if err != nil {
		return nil, err
	}
	refs := make([]tool.Ref, 0, len(rules))
	for _, rule := range rules {
		refs = append(refs, rule.Tool)
	}
	current, err := r.authorities.Fingerprints(ctx, refs)
	if err != nil {
		return nil, err
	}
	view := make([]RuleView, 0, len(rules))
	for _, rule := range rules {
		authority, found := current[rule.Tool]
		entry := RuleView{
			ID: rule.ID, Scope: rule.Scope, Tool: rule.Tool, Subject: rule.Subject, Decision: rule.Decision,
			Stale: !found || authority != rule.SourceFingerprint,
		}
		if rule.Scope == approval.ScopeProject {
			entry.ProjectDir = rule.ScopeKey
		}
		view = append(view, entry)
	}
	return view, nil
}

func (r *RuntimePolicy) SetRule(ctx context.Context, change RuleChange, projectDir string) error {
	authority, found, err := r.authorities.Fingerprint(ctx, change.Tool)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: tool source is missing", approval.ErrInvalidRule)
	}
	// The authority was read here, so the rule binds it without the
	// frozen-versus-current comparison Remember makes for an execution.
	rule, err := approval.RememberRequest{Scope: change.Scope, SessionID: change.SessionID, ProjectDir: projectDir, Tool: change.Tool, SourceFingerprint: authority, Subject: change.Subject, Decision: change.Decision}.Rule()
	if err != nil {
		return err
	}
	return r.put(ctx, rule)
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
