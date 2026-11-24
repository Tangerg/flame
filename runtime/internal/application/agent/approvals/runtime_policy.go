package approvals

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/samber/lo"
)

func NewRuntimePolicy(
	initialDefault approval.Mode,
	store RuleStore,
	modeStore modeStore,
	sessions SessionLookup,
	authorities sourceAuthorities,
	invalidations invalidation.Publish,
) (*RuntimePolicy, error) {
	if !initialDefault.ValidDefault() {
		return nil, fmt.Errorf("%w: %q", approval.ErrInvalidMode, initialDefault)
	}
	for _, required := range []struct {
		name  string
		value any
	}{
		{"source authorities", authorities}, {"rule store", store}, {"session mode store", modeStore}, {"session lookup", sessions}, {"invalidations", invalidations},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("approvals: %s is required", required.name)
		}
	}
	return &RuntimePolicy{
		initialDefault: initialDefault, authorities: authorities, store: store,
		modeStore: modeStore, sessions: sessions, invalidations: invalidations,
	}, nil
}

type modeStore interface {
	DefaultMode(ctx context.Context) (mode approval.Mode, found bool, err error)
	SetDefaultMode(ctx context.Context, mode approval.Mode) error
	PlanModeActive(ctx context.Context, sessionID identity.SessionID) (bool, error)
	StartPlanMode(ctx context.Context, sessionID identity.SessionID) (changed bool, err error)
	EndPlanMode(ctx context.Context, sessionID identity.SessionID, revision uint64) (changed bool, err error)
}

type RuntimePolicy struct {
	initialDefault approval.Mode
	authorities    sourceAuthorities
	modeStore      modeStore
	store          RuleStore
	sessions       SessionLookup
	invalidations  invalidation.Publish
}

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

func (r *RuntimePolicy) Mode(ctx context.Context, sessionID identity.SessionID) (approval.Mode, error) {
	if !sessionID.IsZero() {
		if err := sessionID.Validate(); err != nil {
			return "", fmt.Errorf("%w: %w", approval.ErrInvalidSessionMode, err)
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

func (r *RuntimePolicy) EnterPlanMode(ctx context.Context, sessionID identity.SessionID) (changed bool, err error) {
	if parseErr := sessionID.Validate(); parseErr != nil {
		return false, fmt.Errorf("%w: %w", approval.ErrInvalidSessionMode, parseErr)
	}
	return r.modeStore.StartPlanMode(ctx, sessionID)
}

func (r *RuntimePolicy) ExitPlanMode(ctx context.Context, sessionID identity.SessionID, revision uint64) (restored approval.Mode, changed bool, err error) {
	if parseErr := sessionID.Validate(); parseErr != nil {
		return "", false, fmt.Errorf("%w: %w", approval.ErrInvalidSessionMode, parseErr)
	}
	changed, err = r.modeStore.EndPlanMode(ctx, sessionID, revision)
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
	candidates, err := r.visibleRules(ctx, q.SessionID, q.WorkspaceDir)
	if err != nil {
		return "", false, err
	}
	d, ok, err := approval.Decide(candidates, q)
	if err != nil {
		return "", false, err
	}
	return d, ok, nil
}

func (r *RuntimePolicy) PrepareRule(ctx context.Context, req approval.RememberRequest) (approval.Rule, error) {
	rule, err := req.Rule()
	if err != nil {
		return approval.Rule{}, err
	}
	current, found, err := r.authorities.Fingerprint(ctx, req.Tool)
	if err != nil {
		return approval.Rule{}, err
	}
	if !found || current != req.SourceFingerprint {
		return approval.Rule{}, approval.ErrSourceAuthorityChanged
	}
	return rule, nil
}

func (r *RuntimePolicy) put(ctx context.Context, rule approval.Rule) error {
	if err := r.store.Put(ctx, rule); err != nil {
		return err
	}
	r.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}

type sourceAuthorities interface {
	Fingerprint(context.Context, tool.Ref) (framevalue.Digest, bool, error)
	Fingerprints(context.Context, []tool.Ref) (map[tool.Ref]framevalue.Digest, error)
}
type RuleView struct {
	ID         string
	Scope      approval.Scope
	WorkspaceDir string
	Tool       tool.Ref
	Subject    approval.Subject
	Decision   approval.Decision
	Stale      bool
}

func (r *RuntimePolicy) ListRules(ctx context.Context, sessionID identity.SessionID) ([]RuleView, error) {
	workspaceDir := ""
	if !sessionID.IsZero() {
		sess, err := r.sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		workspaceDir = sess.Workspace().Path()
	}
	rules, err := r.visibleRules(ctx, sessionID, workspaceDir)
	if err != nil {
		return nil, err
	}
	refs := make([]tool.Ref, 0, len(rules))
	for _, rule := range rules {
		refs = append(refs, rule.Tool())
	}
	current, err := r.authorities.Fingerprints(ctx, refs)
	if err != nil {
		return nil, err
	}
	view := make([]RuleView, 0, len(rules))
	for _, rule := range rules {
		authority, found := current[rule.Tool()]
		entry := RuleView{
			ID: rule.ID(), Scope: rule.Scope().Kind(), Tool: rule.Tool(), Subject: rule.Subject(), Decision: rule.Decision(),
			Stale: !found || authority != rule.SourceFingerprint(),
		}
		if rule.Scope().Kind() == approval.ScopeWorkspace {
			entry.WorkspaceDir = rule.Scope().Key()
		}
		view = append(view, entry)
	}
	return view, nil
}

func (r *RuntimePolicy) SetRule(ctx context.Context, change RuleChange) error {
	workspaceDir := ""
	if change.Scope != approval.ScopeGlobal {
		sess, err := r.sessions.Get(ctx, change.SessionID)
		if err != nil {
			return err
		}
		workspaceDir = sess.Workspace().Path()
	}
	authority, found, err := r.authorities.Fingerprint(ctx, change.Tool)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: tool source is missing", approval.ErrInvalidRule)
	}
	rule, err := approval.RememberRequest{Scope: change.Scope, SessionID: change.SessionID, WorkspaceDir: workspaceDir, Tool: change.Tool, SourceFingerprint: authority, Subject: change.Subject, Decision: change.Decision}.Rule()
	if err != nil {
		return err
	}
	return r.put(ctx, rule)
}

func (r *RuntimePolicy) visibleRules(ctx context.Context, sessionID identity.SessionID, workspaceDir string) ([]approval.Rule, error) {
	rules, err := r.store.Visible(ctx, sessionID, workspaceDir, approval.MaximumVisibleRules+1)
	if err != nil {
		return nil, err
	}
	if err := approval.ValidateVisibleRules(rules, sessionID, workspaceDir); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *RuntimePolicy) ForgetRule(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: id is required", approval.ErrInvalidRule)
	}
	if err := r.store.Delete(ctx, id); err != nil {
		return err
	}
	r.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}
