package terminal

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestPluginCommandIdentityExhaustionPreservesExistingCancellationOwner(t *testing.T) {
	existing := commandOperation{id: 1, pluginID: "existing.plugin"}
	registry := commandOperationRegistry{
		next: commandOperationID(math.MaxUint64),
		active: map[commandOperationID]commandOperation{
			existing.id: existing,
		},
	}

	if _, err := registry.reserve("replacement.plugin"); !errors.Is(err, errCommandOperationIdentityExhausted) {
		t.Fatalf("reserve after identity exhaustion error = %v", err)
	}

	if got := registry.active[existing.id]; got != existing {
		t.Fatalf("existing cancellation owner = %+v, want %+v", got, existing)
	}
}

func TestPluginCommandRegistryTakesOnlySelectedOwner(t *testing.T) {
	registry := newCommandOperationRegistry()
	first, err := registry.reserve("first.plugin")
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.reserve("second.plugin")
	if err != nil {
		t.Fatal(err)
	}

	taken := registry.take("first.plugin")
	if len(taken) != 1 || taken[0] != first {
		t.Fatalf("taken operations = %+v, want only %+v", taken, first)
	}
	if got, ok := registry.active[second.id]; !ok || got != second {
		t.Fatalf("independent operation = %+v, present %t; want %+v", got, ok, second)
	}
}

func TestCommandsWithUsefulDefaultsDeclareOptionalArguments(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"workspace": true, "fork": true, "usage": true, "memory": true, "memory-add": true,
		"mcp-tools": true, "diff": true, "browse": true,
	}
	for _, command := range builtinCommands() {
		if !want[command.Descriptor.Name] {
			continue
		}
		delete(want, command.Descriptor.Name)
		if command.Descriptor.Arguments != extensions.OptionalArguments {
			t.Errorf("/%s arguments = %v, want optional", command.Descriptor.Name, command.Descriptor.Arguments)
		}
	}
	for name := range want {
		t.Errorf("defaultable command /%s is not registered", name)
	}
}

func TestBuiltinCommandsHonorNegotiatedFineGrainedCapabilities(t *testing.T) {
	t.Parallel()

	profile := terminalProfileWithFeatures(t, map[string]protocol.FeatureCapability{
		protocol.FeatureGit:           {},
		protocol.FeatureRelocate:      {},
		protocol.FeatureSessionExport: {},
	})
	application := &app{
		runtimeProfile: &profile,
		execution:      executionState{conversation: conversation.New()},
		workspaces:     &workspaceServiceStub{},
		transfers:      outputTransferStub{},
	}
	for name, availability := range map[string]extensions.CommandAvailability{
		"changes":  availableWithGitWorkspaceService(application),
		"relocate": availableForRelocation(application),
		"export":   availableWithSessionTransfer(application),
	} {
		if availability.Enabled() || !strings.Contains(availability.Reason(), "was not negotiated") {
			t.Errorf("%s availability = %+v", name, availability)
		}
	}
}

func TestRuntimeFeatureServicesRequireBothPortAndPublishedCapability(t *testing.T) {
	t.Parallel()

	features := map[string]protocol.FeatureCapability{
		protocol.FeatureGoals:       {},
		protocol.FeatureSkills:      {},
		protocol.FeatureMCP:         {},
		protocol.FeatureSchedules:   {},
		protocol.FeatureAgentMemory: {},
	}
	profile := terminalProfileWithFeatures(t, features)
	application := &app{
		runtimeProfile: &profile,
		goals:          new(goalServiceStub),
		skills:         newSkillServiceStub(),
		mcp:            newMCPServiceStub(),
		schedules:      newScheduleServiceStub(),
		agentMemory:    newAgentMemoryServiceStub(),
	}
	checks := map[string]func(*app) extensions.CommandAvailability{
		protocol.FeatureGoals:       availableWithGoals,
		protocol.FeatureSkills:      availableWithSkills,
		protocol.FeatureMCP:         availableWithMCP,
		protocol.FeatureSchedules:   availableWithSchedules,
		protocol.FeatureAgentMemory: availableWithAgentMemory,
	}
	for feature, check := range checks {
		if availability := check(application); availability.Enabled() || !strings.Contains(availability.Reason(), "was not negotiated") {
			t.Errorf("disabled %s availability = %+v", feature, availability)
		}
		capability := features[feature]
		capability.Enabled = true
		features[feature] = capability
		profile = terminalProfileWithFeatures(t, features)
		if availability := check(application); !availability.Enabled() {
			t.Errorf("enabled %s availability = %+v", feature, availability)
		}
	}
}

func TestMessageCapabilitiesRejectImagesOnlyWhenMultimodalWasNotNegotiated(t *testing.T) {
	t.Parallel()

	application := &app{runtimeProfile: new(terminalProfileWithFeatures(t, map[string]protocol.FeatureCapability{
		protocol.FeatureMultimodal: {Enabled: false},
	}))}
	text := prompt.Message{Attachments: []prompt.Attachment{{Kind: protocol.ContentBlockText}}}
	if err := application.validateMessageCapabilities(text); err != nil {
		t.Fatalf("text attachment: %v", err)
	}
	image := prompt.Message{Attachments: []prompt.Attachment{{Kind: protocol.ContentBlockImage}}}
	if err := application.validateMessageCapabilities(image); err == nil || !strings.Contains(err.Error(), "multimodal") {
		t.Fatalf("image attachment error = %v", err)
	}
}

func TestCommandCatalogRejectsNameAndAliasConflicts(t *testing.T) {
	t.Parallel()
	catalog := newCommandCatalog()
	first := extensions.CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"look"}}
	if err := catalog.add("first", first, func(string) {}, nil); err != nil {
		t.Fatal(err)
	}
	conflicting := extensions.CommandDescriptor{Name: "look", Title: "conflict with an alias"}
	if err := catalog.add("second", conflicting, func(string) {}, nil); err == nil {
		t.Fatal("alias conflict was accepted")
	}
}

func TestSplitCommandArgumentPreservesRemainderAcrossWhitespace(t *testing.T) {
	t.Parallel()
	identity, remainder, ok := splitCommandArgument("  inspect\t{\"depth\": 2}  ")
	if !ok || identity != "inspect" || remainder != `{"depth": 2}` {
		t.Fatalf("split = (%q, %q, %t)", identity, remainder, ok)
	}
	if _, _, ok := splitCommandArgument(" \n\t "); ok {
		t.Fatal("empty argument was accepted")
	}
	if remainder, ok := trimCommandIdentity("review code\tcarefully", "review code"); !ok || remainder != "carefully" {
		t.Fatalf("trim identity = (%q, %t)", remainder, ok)
	}
	if _, ok := trimCommandIdentity("reviewer", "review"); ok {
		t.Fatal("partial token matched a complete identity")
	}
}

func TestBuiltinCommandsOwnTheirCategoryAndAvailabilityPolicy(t *testing.T) {
	t.Parallel()
	wantCategories := map[string][]string{
		commandCategoryApplication: {"quit"},
		commandCategoryTranscript:  {"help", "shortcuts", "clear", "find", "next", "previous", "queue", "details", "view", "copy-last", "export", "feedback"},
		commandCategorySessions:    {"sessions", "timeline", "new", "workspace", "relocate", "rename", "fork", "rollback", "import"},
		commandCategoryComposer:    {"attach", "detach", "attachments", "stash", "stashes", "stash-apply", "stash-delete", "editor"},
		commandCategoryRuntime:     {"tools", "tool-invoke", "model", "models", "usage", "roles", "utility", "embedding", "providers", "provider-test", "provider-config", "approval", "status", "rules", "rule-delete", "steer", "goal", "goal-start", "goal-update", "goal-clear", "goal-stop", "goal-resume", "hooks", "hooks-trust", "hooks-revoke"},
		commandCategoryAutomation:  {"schedules", "schedule-create", "schedule-edit", "schedule-enable", "schedule-disable", "schedule-run", "schedule-delete"},
		commandCategoryContext:     {"agent-docs", "memory", "memory-add", "memory-edit", "memory-pin", "memory-unpin", "memory-approve", "memory-reject", "memory-delete", "skills", "skill-library", "skill-proposals", "skill-archive", "skill-restore", "skill-approve", "skill-reject"},
		commandCategoryConnections: {"mcp-tool", "mcp", "mcp-tools", "mcp-create", "mcp-edit", "mcp-probe", "mcp-delete", "mcp-reconnect", "mcp-auth"},
		commandCategoryWorkspace:   {"workspaces", "changes", "diff", "preview", "grep", "browse", "read"},
		commandCategoryExtensions:  {"plugins", "reload", "unload"},
	}
	wantGuard := map[string]bool{
		"clear": true, "view": true, "export": true,
		"sessions": true, "new": true, "workspace": true, "relocate": true, "rename": true, "fork": true, "rollback": true, "import": true,
		"stash": true, "stash-apply": true, "editor": true,
		"workspaces": true, "changes": true, "diff": true, "preview": true, "grep": true, "browse": true, "read": true,
		"usage": true, "roles": true, "utility": true, "embedding": true, "providers": true, "provider-test": true, "provider-config": true,
		"steer": true, "goal": true, "goal-start": true, "goal-update": true, "goal-clear": true, "goal-stop": true, "goal-resume": true,
		"skills": true, "skill-library": true, "skill-proposals": true, "skill-archive": true, "skill-restore": true, "skill-approve": true, "skill-reject": true,
		"memory": true, "memory-add": true, "memory-edit": true, "memory-pin": true, "memory-unpin": true, "memory-approve": true, "memory-reject": true, "memory-delete": true,
		"mcp-tool": true, "mcp": true, "mcp-tools": true, "mcp-create": true, "mcp-edit": true, "mcp-probe": true, "mcp-delete": true, "mcp-reconnect": true, "mcp-auth": true,
		"schedules": true, "schedule-create": true, "schedule-edit": true, "schedule-enable": true, "schedule-disable": true, "schedule-run": true, "schedule-delete": true,
		"tools": true, "tool-invoke": true,
		"agent-docs": true,
		"hooks":      true, "hooks-trust": true, "hooks-revoke": true,
		"feedback": true,
	}

	seen := make(map[string]struct{})
	gotCategories := make(map[string][]string)
	for _, command := range builtinCommands() {
		if err := command.validate(); err != nil {
			t.Fatalf("validate /%s: %v", command.Descriptor.Name, err)
		}
		for _, identity := range command.Descriptor.Identities() {
			if _, duplicate := seen[identity]; duplicate {
				t.Fatalf("command identity %q is duplicated", identity)
			}
			seen[identity] = struct{}{}
		}
		gotCategories[command.Descriptor.Category] = append(gotCategories[command.Descriptor.Category], command.Descriptor.Name)
		if got := command.Available != nil; got != wantGuard[command.Descriptor.Name] {
			t.Errorf("/%s availability policy present = %t, want %t", command.Descriptor.Name, got, wantGuard[command.Descriptor.Name])
		}
	}
	for category, want := range wantCategories {
		if got := gotCategories[category]; !slices.Equal(got, want) {
			t.Errorf("%s commands = %v, want %v", category, got, want)
		}
		delete(gotCategories, category)
	}
	for category, commands := range gotCategories {
		t.Errorf("unexpected category %q with commands %v", category, commands)
	}
}

func TestCommandCatalogRanksAnExactAliasAheadOfFuzzyNames(t *testing.T) {
	t.Parallel()
	catalog := newCommandCatalog()
	if err := catalog.add("test", extensions.CommandDescriptor{Name: "goal-resume", Title: "resume goal"}, func(string) {}, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.add("test", extensions.CommandDescriptor{Name: "sessions", Title: "resume session", Aliases: []string{"resume"}}, func(string) {}, nil); err != nil {
		t.Fatal(err)
	}

	found := catalog.find("resume")
	if len(found) == 0 || found[0].Command.Name != "sessions" {
		t.Fatalf("find exact alias = %v, want sessions first", found)
	}
}
