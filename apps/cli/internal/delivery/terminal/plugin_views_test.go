package terminal

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"
	"github.com/Tangerg/qimu/portable"
)

type noteRow struct {
	ID       string
	Content  string
	Revision int64
}

// notesPlugin serves one declared terminal view over an in-memory note list.
type notesPlugin struct {
	mu      sync.Mutex
	notes   []noteRow
	updates chan map[string]any
}

func (p *notesPlugin) setContent(id, content string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.notes {
		if p.notes[i].ID == id {
			p.notes[i].Content = content
			p.notes[i].Revision++
		}
	}
}

func (p *notesPlugin) respond(request protocol.CallPluginRequest) (*protocol.CallPluginResponse, error) {
	switch request.Method {
	case "notes/list":
		p.mu.Lock()
		output, err := json.Marshal(map[string]any{"Rows": p.notes})
		p.mu.Unlock()
		return &protocol.CallPluginResponse{Output: string(output)}, err
	case "notes/update":
		var input map[string]any
		if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
			return nil, err
		}
		p.setContent(input["ID"].(string), input["Content"].(string))
		p.updates <- input
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	}
	return nil, nil
}

func notesInstallation(t *testing.T) *protocol.PluginInstallation {
	t.Helper()
	contract, err := portable.NewCapability(
		portable.CapabilityConfig{Reference: "notes", Composition: portable.Selection, Scope: extension.ScopeInstallation},
		portable.MethodDescriptor{Name: "notes/list", Kind: portable.Query, Target: extension.ScopeInstallation,
			Input:  jsontext.Value(`{"type":"object","additionalProperties":false,"required":["CWD"],"properties":{"CWD":{"type":"string"}}}`),
			Output: jsontext.Value(`{"type":"object","properties":{"Rows":{"type":"array","items":{"type":"object"}}}}`)},
		portable.MethodDescriptor{Name: "notes/update", Kind: portable.Mutation, Target: extension.ScopeInstallation,
			Input:  jsontext.Value(`{"type":"object","additionalProperties":false,"required":["ID","Revision","Content"],"properties":{"ID":{"type":"string"},"Revision":{"type":"integer"},"Content":{"type":"string"}}}`),
			Output: jsontext.Value(`{"type":"object"}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	view := protocol.PluginTerminalView{
		ID: "notes", Title: "Notes", Contribution: "notes-provider",
		List: protocol.PluginTerminalList{
			Method: "notes/list", Rows: "Rows", Key: "ID",
			Columns: []protocol.PluginTerminalColumn{{Title: "Content", Field: "Content"}},
			Input:   []protocol.PluginTerminalInput{{Path: "CWD", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingContext, Context: extension.TerminalContextWorkspace}}},
		},
		Actions: []protocol.PluginTerminalAction{{
			ID: "edit", Title: "Edit", Method: "notes/update", Target: extension.TerminalActionRow,
			Input: []protocol.PluginTerminalInput{
				{Path: "ID", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingRow, Field: "ID"}},
				{Path: "Revision", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingRow, Field: "Revision"}},
				{Path: "Content", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingForm, Form: &protocol.PluginTerminalFormField{
					Title: "Content", Kind: extension.TerminalFormText,
					Default: &protocol.PluginTerminalBinding{Source: extension.TerminalBindingRow, Field: "Content"},
				}}},
			},
		}},
	}
	return &protocol.PluginInstallation{
		ID: "12345678-1234-1234-1234-123456789abc", Enabled: true,
		Selected: protocol.PluginRelease{
			Name: "flame.notes", Version: "1.0.0", Digest: strings.Repeat("c", 64),
			TerminalViews: []protocol.PluginTerminalView{view},
			Contributions: []protocol.PluginContribution{{ID: "notes-provider", Contract: string(contract.Identity().JSON())}},
		},
		Presentation: protocol.PluginPresentationAdmitted,
		Realization:  protocol.PluginRealization{Type: protocol.PluginRealizationAvailable},
	}
}

func runNotesView(t *testing.T) (*notesPlugin, *installedPluginCatalog, *runtimeChangeSourceStub, *programtest.Host, func()) {
	t.Helper()
	plugin := &notesPlugin{notes: []noteRow{{ID: "note_release", Content: "confirm release steps", Revision: 1}}, updates: make(chan map[string]any, 1)}
	catalog := &installedPluginCatalog{installation: notesInstallation(t), respond: plugin.respond}
	source := runtimeResourceChangeSource(protocol.TopicPluginDataChanged)
	host, stop := runUIWithRuntimeServices(t, Config{
		RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench,
		Runtime: runtimefixture.New(), Plugins: catalog, Changes: source, Workspace: "/workspace",
	})
	host.Shows(t, "Ask flame")
	return plugin, catalog, source, host, stop
}

func TestPluginViewListsRowsThroughTheDeclaredContract(t *testing.T) {
	_, catalog, _, host, stop := runNotesView(t)
	host.Type("/notes")
	host.Press(input.Enter)
	host.Shows(t, "note_release")
	host.Shows(t, "confirm release steps")
	host.Shows(t, "/notes edit <id>")
	catalog.mu.Lock()
	list := catalog.calls[len(catalog.calls)-1]
	catalog.mu.Unlock()
	if list.Method != "notes/list" || list.Input != `{"CWD":"/workspace"}` || list.InstallationID != "12345678-1234-1234-1234-123456789abc" {
		t.Fatalf("list call = %+v", list)
	}
	stop()
}

func TestPluginViewReaderFollowsPluginDataChanges(t *testing.T) {
	plugin, _, source, host, stop := runNotesView(t)
	host.Type("/notes")
	host.Press(input.Enter)
	host.Shows(t, "confirm release steps")
	plugin.setContent("note_release", "confirm externally revised release steps")
	source.events <- changefeed.Event{Type: protocol.RuntimePluginDataChanged, Sequence: 1}
	awaitSignal(t, source.applied, "plugins.dataChanged delivery")
	host.Shows(t, "confirm externally revised release steps")
	stop()
}

func TestPluginViewActionFillsTheFormFromTheSelectedRow(t *testing.T) {
	plugin, _, _, host, stop := runNotesView(t)
	host.Type("/notes edit note_rel")
	host.Press(input.Enter)
	host.Shows(t, "Edit · Notes · note_release")
	host.Shows(t, "confirm release steps")
	host.Type(" today")
	host.Press(input.Enter)
	update := awaitValue(t, plugin.updates, "note update")
	if update["ID"] != "note_release" || update["Revision"] != float64(1) || update["Content"] != "confirm release steps today" {
		t.Fatalf("update input = %v", update)
	}
	host.Shows(t, "Notes · edit done")
	stop()
}
