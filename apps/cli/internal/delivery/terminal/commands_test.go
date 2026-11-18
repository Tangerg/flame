package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/base"
)

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

func TestBuiltinCommandsHonorNegotiatedFineGrainedCapabilities(t *testing.T) {
	t.Parallel()

	profile := terminalProfileWithFeatures(t, map[string]protocol.FeatureCapability{protocol.FeatureGit: {}, protocol.FeatureRelocate: {}, protocol.FeatureSessionExport: {}})
	application := &app{composer: testPromptComposer(t),
		runtimeProfile: &profile,
		runs:           &runs{execution: executionState{conversation: conversation.New()}},
		workspaces:     &workspaceServiceStub{},
		lifecycle:      &sessionLifecycle{transfers: outputTransferStub{}},
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

	features := map[string]protocol.FeatureCapability{protocol.FeatureGoals: {}, protocol.FeatureSkills: {}, protocol.FeatureMCP: {}, protocol.FeaturePlugins: {}}
	profile := terminalProfileWithFeatures(t, features)
	application := &app{composer: testPromptComposer(t),
		runtimeProfile: &profile,
		screens: screens{
			goals:  &goalScreens{service: new(goalServiceStub)},
			skills: &skillScreens{service: newSkillServiceStub()},
			mcp:    &mcpScreens{service: newMCPServiceStub()},
		},
	}
	checks := []struct {
		feature string
		check   func(*app) extensions.CommandAvailability
	}{
		{protocol.FeatureGoals, availableWithGoals},
		{protocol.FeatureSkills, availableWithSkills},
		{protocol.FeatureMCP, availableWithMCP},
	}
	for _, entry := range checks {
		feature, check := entry.feature, entry.check
		capability := features[feature]
		capability.Enabled = false
		features[feature] = capability
		profile = terminalProfileWithFeatures(t, features)
		if availability := check(application); availability.Enabled() || !strings.Contains(availability.Reason(), "was not negotiated") {
			t.Errorf("disabled %s availability = %+v", feature, availability)
		}
		capability = features[feature]
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

	application := &app{composer: testPromptComposer(t), runtimeProfile: new(terminalProfileWithFeatures(t, map[string]protocol.FeatureCapability{protocol.FeatureMultimodal: {Enabled: false}}))}
	text := authoring.Message{Attachments: []authoring.Attachment{{Kind: protocol.ContentBlockText}}}
	if err := application.validateMessageCapabilities(text); err != nil {
		t.Fatalf("text attachment: %v", err)
	}
	image := authoring.Message{Attachments: []authoring.Attachment{{Kind: protocol.ContentBlockImage}}}
	if err := application.validateMessageCapabilities(image); err == nil || !strings.Contains(err.Error(), "multimodal") {
		t.Fatalf("image attachment error = %v", err)
	}
}

func TestCommandCatalogRejectsNameAndAliasConflicts(t *testing.T) {
	t.Parallel()
	catalog := newCommandCatalog()
	first := extensions.CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"look"}}
	if err := catalog.add("first", first, func(string, func()) {}, nil); err != nil {
		t.Fatal(err)
	}
	conflicting := extensions.CommandDescriptor{Name: "look", Title: "conflict with an alias"}
	if err := catalog.add("second", conflicting, func(string, func()) {}, nil); err == nil {
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

func TestCommandCatalogRanksAnExactAliasAheadOfFuzzyNames(t *testing.T) {
	t.Parallel()
	catalog := newCommandCatalog()
	if err := catalog.add("test", extensions.CommandDescriptor{Name: "goal-resume", Title: "resume goal"}, func(string, func()) {}, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.add("test", extensions.CommandDescriptor{Name: "sessions", Title: "resume session", Aliases: []string{"resume"}}, func(string, func()) {}, nil); err != nil {
		t.Fatal(err)
	}

	found := catalog.find("resume")
	if len(found) == 0 || found[0].Command.Name != "sessions" {
		t.Fatalf("find exact alias = %v, want sessions first", found)
	}
}

func TestRejectedSlashCommandsPreserveTheEditorAndAttachments(t *testing.T) {
	for _, test := range []struct {
		name         string
		text         string
		arguments    extensions.ArgumentMode
		availability func(*app) extensions.CommandAvailability
	}{
		{name: "unknown", text: "/missing argument"},
		{name: "unavailable", text: "/inspect argument", arguments: extensions.RequiredArguments, availability: func(*app) extensions.CommandAvailability { return extensions.CommandUnavailable("offline") }},
		{name: "missing argument", text: "/inspect", arguments: extensions.RequiredArguments},
		{name: "extra argument", text: "/inspect argument"},
		{name: "availability panic", text: "/inspect", availability: func(*app) extensions.CommandAvailability { panic("availability failed") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			application := &app{composer: testPromptComposer(t),
				status: &statusView{}, runs: &runs{execution: executionState{conversation: conversation.New()}},
				commands: newCommandCatalog(),
			}
			if err := application.commands.add("test", extensions.CommandDescriptor{Name: "inspect", Title: "Inspect", Arguments: test.arguments}, func(string, func()) { t.Fatal("rejected command executed") }, test.availability); err != nil {
				t.Fatal(err)
			}
			editor := application.composer.Controller()
			attachment := editor.InsertElement(fileElement, "@context.txt")
			application.composer.chips = map[base.ElementID]authoring.Attachment{attachment.ID: {ID: "context", Kind: protocol.ContentBlockText, Name: "context.txt", MimeType: "text/plain"}}
			editor.Insert(test.text)
			editor.SetCursor(0, 3)
			revision := editor.Revision()
			line, column := editor.Cursor()
			message, err := application.composer.message()
			if err != nil {
				t.Fatal(err)
			}
			application.submit()
			after, err := application.composer.message()
			if err != nil {
				t.Fatal(err)
			}
			gotLine, gotColumn := editor.Cursor()
			if !after.Equal(message) || editor.Revision() != revision || gotLine != line || gotColumn != column {
				t.Fatalf("rejected command changed draft or cursor: before=%+v after=%+v", message, after)
			}
		})
	}
}
