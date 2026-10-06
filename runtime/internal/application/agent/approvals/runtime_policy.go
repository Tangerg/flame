package approvals

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// NewRuntimePolicy constructs permission policy over durable permission modes
// and remembered rules. initialDefault is the Runtime default until a user
// chooses one; only a Session may enter Plan mode.
func NewRuntimePolicy(
	initialDefault approval.Mode,
	store RuleStore,
	modeStore ModeStore,
	authorities SourceAuthorities,
	invalidations invalidation.Publish,
) (*RuntimePolicy, error) {
	if !initialDefault.ValidDefault() {
		return nil, fmt.Errorf("%w: %q", approval.ErrInvalidMode, initialDefault)
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
	return &RuntimePolicy{
		initialDefault: initialDefault, authorities: authorities, store: store,
		modeStore: modeStore, invalidations: invalidations,
	}, nil
}

// ModeStore persists permission modes: the Runtime default a user chose, and
// explicit per-session Plan mode. A missing session row means the session
// follows the Runtime default; a missing default means none was chosen.
// Implementations validate ownership and decoded modes at their persistence
// boundary.
//
// Retiring a deleted Session's state is not here: the session write-set owns
// that, transactionally with everything else the delete removes, through its
// own cleaner port.
type ModeStore interface {
	DefaultMode(ctx context.Context) (mode approval.Mode, found bool, err error)
	SetDefaultMode(ctx context.Context, mode approval.Mode) error
	PlanModeActive(ctx context.Context, sessionID string) (bool, error)
	StartPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
	EndPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
}

// RuntimePolicy combines two policy facts consumed together at the tool-call
// boundary: session-effective permission mode and remembered approval rules.
// The Runtime default is durable so a restart never loosens or tightens the
// stance a user chose.
type RuntimePolicy struct {
	initialDefault approval.Mode
	authorities    SourceAuthorities
	modeStore      ModeStore
	store          RuleStore
	invalidations  invalidation.Publish
}

// DefaultMode returns the runtime fallback used by sessions without an explicit
// mode row.
func (r *RuntimePolicy) DefaultMode(ctx context.Context) (approval.Mode, error) {
	mode, found, err := r.modeStore.DefaultMode(ctx)
	if err != nil {
		return "", err
	}
	if !found {
		return r.initialDefault, nil
	}
	return mode, nil
}

// SetDefaultMode changes the runtime fallback. Plan mode is session-only and is
// therefore rejected here.
func (r *RuntimePolicy) SetDefaultMode(ctx context.Context, mode approval.Mode) error {
	if !mode.ValidDefault() {
		return fmt.Errorf("%w: %q", approval.ErrInvalidMode, mode)
	}
	if err := r.modeStore.SetDefaultMode(ctx, mode); err != nil {
		return err
	}
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
