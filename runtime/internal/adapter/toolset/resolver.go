package toolset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
	oteltool "github.com/Tangerg/scope/otel/tool"
	"github.com/samber/lo"
)

type Resolver struct {
	goalAvailable func(context.Context) (bool, error)
	telemetry     oteltool.Middleware
	skillSources  promptsource.SkillSources
	skillsUserDir string
	skillUsage    builtin.SkillUsageRecorder
	codeIntel     CodeIntelligence
	readTracker   *readTracker
	pathLocker    *keylock.Set
	shell         []toolcontract.Tool
	planAvailable func(context.Context) (bool, error)
	staticSpecs   []staticSpec

	lateMu     sync.RWMutex
	createGoal toolcontract.Tool

	mcp atomic.Pointer[mcpCatalog]

	mcpToolDisabled func(domaintool.Ref) bool
	restoreMCP      func(context.Context, []domaintool.Ref, []plugin.Activation) ([]toolcontract.Tool, func() error, error)
}

type audience uint8

const (
	audienceBoth audience = iota
	audienceRoot
)

func (a audience) includes(group domaintool.Group) bool {
	return a == audienceBoth || group == domaintool.GroupRoot
}

type placement uint8

const (
	afterSkill placement = iota
	interactionTail
	rootTail
)

type availability uint8

const (
	alwaysAvailable availability = iota
	whenPlanAvailable
	goalRunsOnly
)

func (a availability) admits(ctx context.Context, planAvailable bool) bool {
	switch a {
	case whenPlanAvailable:
		return planAvailable
	case goalRunsOnly:
		_, owned := executionctx.GoalIncarnationID(ctx)
		return owned
	default:
		return true
	}
}

type staticSpec struct {
	tool         toolcontract.Tool
	audience     audience
	placement    placement
	availability availability
	deferred     bool
}

type resolverDeps struct {
	GoalAvailable   func(context.Context) (bool, error)
	PlanAvailable   func(context.Context) (bool, error)
	SkillSources    promptsource.SkillSources
	SkillsUserDir   string
	SkillUsage      builtin.SkillUsageRecorder
	Shell           []toolcontract.Tool
	AskUser         toolcontract.Tool
	EnterPlan       toolcontract.Tool
	ExitPlan        toolcontract.Tool
	Plan            toolcontract.Tool
	ToolResult      toolcontract.Tool
	GoalGet         toolcontract.Tool
	GoalReport      toolcontract.Tool
	ProposeSkill    toolcontract.Tool
	CodeIntel       CodeIntelligence
	ReadTracker     *readTracker
	MCPToolDisabled func(domaintool.Ref) bool
	RestoreMCP      func(context.Context, []domaintool.Ref, []plugin.Activation) ([]toolcontract.Tool, func() error, error)
}

func newResolver(d resolverDeps) (*Resolver, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"read tracker", d.ReadTracker}, {"code intelligence", d.CodeIntel}, {"Skill usage recorder", d.SkillUsage},
		{"Goal availability", d.GoalAvailable}, {"Plan availability", d.PlanAvailable},
		{"MCP exposure policy", d.MCPToolDisabled}, {"MCP restoration", d.RestoreMCP},
		{"ask_user", d.AskUser}, {"enter_plan_mode", d.EnterPlan}, {"exit_plan_mode", d.ExitPlan}, {"set_plan", d.Plan},
		{"read_tool_result", d.ToolResult}, {"get_goal", d.GoalGet}, {"report_goal_outcome", d.GoalReport}, {"propose_skill", d.ProposeSkill},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("toolset: resolver %s is required", required.name)
		}
	}
	if err := d.SkillSources.Validate(); err != nil {
		return nil, fmt.Errorf("toolset: %w", err)
	}
	telemetry, err := oteltool.NewMiddleware(oteltool.MiddlewareConfig{})
	if err != nil {
		return nil, fmt.Errorf("toolset: instrument tool calls: %w", err)
	}
	resolver := &Resolver{
		telemetry:     telemetry,
		skillsUserDir: d.SkillsUserDir, skillSources: d.SkillSources,
		skillUsage:    d.SkillUsage,
		shell:         slices.Clone(d.Shell),
		goalAvailable: d.GoalAvailable,
		planAvailable: d.PlanAvailable,
		staticSpecs: []staticSpec{
			{tool: d.AskUser, audience: audienceBoth, placement: interactionTail},
			{tool: d.EnterPlan, audience: audienceRoot, placement: interactionTail, availability: whenPlanAvailable},
			{tool: d.ExitPlan, audience: audienceRoot, placement: interactionTail, availability: whenPlanAvailable},
			{tool: d.Plan, audience: audienceRoot, placement: afterSkill, availability: whenPlanAvailable},
			{tool: d.ToolResult, audience: audienceBoth, placement: afterSkill},
			{tool: d.ProposeSkill, audience: audienceRoot, placement: afterSkill, deferred: true},
			{tool: d.GoalGet, audience: audienceRoot, placement: rootTail},
			{tool: d.GoalReport, audience: audienceRoot, placement: rootTail, availability: goalRunsOnly},
		},
		codeIntel:       d.CodeIntel,
		readTracker:     d.ReadTracker,
		pathLocker:      keylock.NewSet(),
		mcpToolDisabled: d.MCPToolDisabled,
		restoreMCP:      d.RestoreMCP,
	}
	return resolver, nil
}

func (r *Resolver) appendStatic(
	ctx context.Context,
	into *manifestBuilder,
	at placement,
	group domaintool.Group, planAvailable bool,
) {
	for _, spec := range r.staticSpecs {
		if spec.placement != at || !spec.audience.includes(group) || !spec.availability.admits(ctx, planAvailable) {
			continue
		}
		if spec.deferred {
			into.deferTools(spec.tool)
		} else {
			into.direct(spec.tool)
		}
	}
}

func (r *Resolver) UseCreateGoalTool(tool toolcontract.Tool) {
	r.lateMu.Lock()
	defer r.lateMu.Unlock()
	r.createGoal = tool
}

func (r *Resolver) createGoalTool() toolcontract.Tool {
	r.lateMu.RLock()
	defer r.lateMu.RUnlock()
	return r.createGoal
}

type mcpCatalog struct {
	tools   []toolcontract.Tool
	pending []mcpserver.ID
}

func (r *Resolver) mcpTools(ctx context.Context, retained *Retained) ([]toolcontract.Tool, []mcpserver.ID, func() error, error) {
	snapshot := r.mcp.Load()
	if snapshot == nil {
		snapshot = &mcpCatalog{}
	}
	if retained == nil {
		tools, err := r.exposedMCPTools(snapshot.tools, nil)
		return tools, slices.Clone(snapshot.pending), nil, err
	}
	users := make([]domaintool.Ref, 0, len(retained.MCP))
	var installed []domaintool.Ref
	for _, ref := range retained.MCP {
		server, _, _ := ref.MCP()
		if _, managed := server.Origin().Installation(); managed {
			installed = append(installed, ref)
		} else {
			users = append(users, ref)
		}
	}
	current, err := r.exposedMCPTools(snapshot.tools, users)
	if err != nil {
		return nil, nil, nil, err
	}
	var pending []mcpserver.ID
	for _, source := range snapshot.pending {
		if slices.ContainsFunc(users, func(ref domaintool.Ref) bool {
			server, _, _ := ref.MCP()
			return server == source
		}) {
			pending = append(pending, source)
		}
	}
	if len(installed) == 0 {
		return current, pending, nil, nil
	}
	original, release, err := r.restoreMCP(ctx, installed, cloneManifestBindings(retained.Bindings))
	if err != nil {
		if release != nil {
			err = errors.Join(err, release())
		}
		return nil, nil, nil, err
	}
	if release == nil {
		return nil, nil, nil, errors.New("toolset: original MCP restoration requires resource ownership")
	}
	if err := validateRestoredMCP(original, installed, retained.Bindings); err != nil {
		return nil, nil, nil, errors.Join(err, release())
	}
	original, err = r.exposedMCPTools(original, installed)
	if err != nil {
		return nil, nil, nil, errors.Join(err, release())
	}
	return append(slices.Clone(current), original...), pending, release, nil
}

func validateRestoredMCP(values []toolcontract.Tool, references []domaintool.Ref, bindings []plugin.Activation) error {
	if len(values) != len(references) {
		return fmt.Errorf("%w: original MCP tools are incomplete", plugin.ErrUnavailable)
	}
	var seen []domaintool.Ref
	for _, executable := range values {
		ref, found, err := mcp.IdentifyTool(executable)
		if err != nil {
			return err
		}
		if !found || !slices.Contains(references, ref) || slices.Contains(seen, ref) {
			return fmt.Errorf("%w: original MCP tool identity changed", plugin.ErrStale)
		}
		seen = append(seen, ref)
		dependency, installed, err := InstallationDependency(executable)
		if err != nil {
			return err
		}
		if !installed {
			return fmt.Errorf("%w: restored MCP tool is not installed", plugin.ErrInvalid)
		}
		server, _, _ := ref.MCP()
		original, err := plugins.OriginalActivation(bindings, dependency.InstallationID, dependency.Digest, server.Name())
		if err != nil {
			return err
		}
		actual, err := InstallationActivation(executable)
		if err != nil {
			return err
		}
		if !actual.Equal(original) {
			return fmt.Errorf("%w: original MCP setup binding changed", plugin.ErrStale)
		}
	}
	return nil
}

func (r *Resolver) mcpSnapshot() []toolcontract.Tool {
	p := r.mcp.Load()
	if p == nil {
		return nil
	}
	return p.tools
}

func (r *Resolver) exposedMCPTools(values []toolcontract.Tool, capturedMCP []domaintool.Ref) ([]toolcontract.Tool, error) {
	var out []toolcontract.Tool
	for i, tool := range values {
		ref, found, err := mcp.IdentifyTool(tool)
		if err != nil {
			return nil, fmt.Errorf("toolset: resolve MCP tool identity: %w", err)
		}
		if !found {
			return nil, errors.New("toolset: MCP tool has no source identity")
		}
		if (capturedMCP != nil && !slices.Contains(capturedMCP, ref)) || r.mcpToolDisabled(ref) {
			if out == nil {
				out = append(make([]toolcontract.Tool, 0, len(values)-1), values[:i]...)
			}
			continue
		}
		if out != nil {
			out = append(out, tool)
		}
	}
	if out == nil {
		return values, nil
	}
	return out, nil
}

func (r *Resolver) SetMCPTools(tools []toolcontract.Tool, pending []mcpserver.ID) {
	r.mcp.Store(&mcpCatalog{tools: slices.Clone(tools), pending: slices.Clone(pending)})
}

// A replaced or compacted context must earn fresh read-before-write authority.
func (r *Resolver) ForgetSessionContext(sessionID identity.SessionID) {
	r.readTracker.forgetSession(sessionID)
}

// Sessions may share a working tree, so invalidating only the requesting Session
// would leave sibling authority stale.
func (r *Resolver) ForgetWorkspace(root string) {
	r.readTracker.forgetWorkspace(root)
}

func (r *Resolver) toolsForCWD(cwd string) (cwdTools, error) {
	return openCWDTools(cwd, r.codeIntel, r.readTracker, r.pathLocker)
}

func (r *Resolver) Manifest(ctx context.Context, group domaintool.Group, retained *Retained) (Manifest, error) {
	if err := retained.validate(); err != nil {
		return Manifest{}, err
	}
	resolved, err := r.resolve(ctx, group, retained)
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := resolved.manifest(r.telemetry)
	if err == nil {
		err = retained.validateManifest(manifest)
	}
	if err != nil {
		return Manifest{}, errors.Join(err, resolved.close())
	}
	return manifest, nil
}

func (r *Resolver) resolve(ctx context.Context, group domaintool.Group, retained *Retained) (_ manifestBuilder, err error) {
	if !group.Valid() {
		return manifestBuilder{}, fmt.Errorf("toolset: unsupported Tool group %q", group)
	}
	cwd, attached := executionctx.CWD(ctx)
	if !attached {
		return manifestBuilder{}, errors.New("toolset: resolve Tools without an attached Run workspace")
	}
	localTools, err := r.toolsForCWD(cwd)
	if err != nil {
		return manifestBuilder{}, err
	}
	release := localTools.close
	defer func() {
		if err != nil {
			err = errors.Join(err, release())
		}
	}()
	tools := manifestBuilder{close: localTools.close}
	tools.direct(localTools.readSearch...)
	tools.direct(localTools.edit)
	tools.direct(localTools.applyPatch)
	mcpTools, pending, closeMCP, err := r.mcpTools(ctx, retained)
	if err != nil {
		return manifestBuilder{}, err
	}
	if closeMCP != nil {
		release = sync.OnceValue(func() error { return errors.Join(closeMCP(), localTools.close()) })
		tools.close = release
	}
	tools.deferred = append(tools.deferred, mcpTools...)
	tools.pendingMCP = pending
	tools.direct(r.shell...)
	if err := r.resolveSkillTools(ctx, cwd, retained, &tools); err != nil {
		return manifestBuilder{}, err
	}
	planAvailable := true
	if group == domaintool.GroupRoot {
		planAvailable, err = r.planAvailable(ctx)
		if err != nil {
			return manifestBuilder{}, err
		}
	}
	r.appendStatic(ctx, &tools, afterSkill, group, planAvailable)
	r.appendStatic(ctx, &tools, interactionTail, group, planAvailable)
	if group == domaintool.GroupRoot {
		if err := r.appendRootTools(ctx, &tools, planAvailable); err != nil {
			return manifestBuilder{}, err
		}
	}
	if err := tools.bindDiscovery(retained); err != nil {
		return manifestBuilder{}, err
	}
	return tools, nil
}

func (r *Resolver) appendRootTools(ctx context.Context, tools *manifestBuilder, planAvailable bool) error {
	if createGoal := r.createGoalTool(); createGoal != nil {
		available, err := r.goalAvailable(ctx)
		if err != nil {
			return err
		}
		if available {
			tools.direct(createGoal)
		}
	}
	r.appendStatic(ctx, tools, rootTail, domaintool.GroupRoot, planAvailable)
	return nil
}

func (m *manifestBuilder) bindDiscovery(retained *Retained) error {
	if err := m.bindModelNames(retained); err != nil {
		return err
	}
	if len(m.deferred) == 0 {
		return nil
	}
	search, err := NewDiscovery(m.deferred)
	if err != nil {
		return fmt.Errorf("toolset: resolve search_tools: %w", err)
	}
	m.direct(search)
	m.discovery = search
	return nil
}

func (r *Resolver) resolveSkillTools(ctx context.Context, cwd string, retained *Retained, tools *manifestBuilder) error {
	var resources []plugin.Dependency
	sources := r.skillSources
	capture := &promptsource.SkillCapture{}
	if retained != nil {
		resources = append([]plugin.Dependency{}, retained.Resources...)
		capture.Restore = true
		capture.Command = retained.SkillDiscovery
	}
	sources.Capture = capture
	skillTools, dependencies, err := builtin.BuildReaders(ctx, cwd, r.skillsUserDir, r.skillUsage, sources, resources)
	tools.skillDiscovery = *capture
	if err != nil {
		return fmt.Errorf("toolset: resolve skill tools: %w", err)
	}
	for index, skillTool := range skillTools {
		skillTools[index] = withDefiniteOutcome(skillTool)
	}
	tools.installations = dependencies
	tools.deferTools(skillTools...)

	return nil
}
