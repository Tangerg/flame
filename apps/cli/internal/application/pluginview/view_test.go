package pluginview

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/qimu/portable"
)

func testView(t *testing.T) View {
	t.Helper()
	contract, err := portable.NewCapability(
		portable.CapabilityConfig{Reference: "memory", Composition: portable.Selection, Scope: extension.ScopeInstallation},
		portable.MethodDescriptor{Name: "memory/list", Kind: portable.Query, Target: extension.ScopeInstallation,
			Input:  jsontext.Value(`{"type":"object","additionalProperties":false,"required":["Scope","CWD","Cursor"],"properties":{"Scope":{"enum":["project","user"],"type":"string"},"CWD":{"type":"string"},"Cursor":{"type":"string"}}}`),
			Output: jsontext.Value(`{"type":"object","properties":{"Rows":{"type":"array","items":{"type":"object"}},"NextCursor":{"type":"string"}}}`)},
		portable.MethodDescriptor{Name: "memory/update", Kind: portable.Mutation, Target: extension.ScopeInstallation,
			Input:  jsontext.Value(`{"type":"object","additionalProperties":false,"required":["ID","Revision","Model"],"properties":{"ID":{"type":"string"},"Revision":{"type":"integer"},"Pinned":{"type":"boolean"},"Model":{"type":"object","additionalProperties":false,"properties":{"Provider":{"type":"string"}}}}}`),
			Output: jsontext.Value(`{"type":"object"}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	scope := protocol.PluginTerminalBinding{Source: extension.TerminalBindingForm, Form: &protocol.PluginTerminalFormField{Title: "Scope", Kind: extension.TerminalFormChoice, Options: []string{"project", "user"}, Default: &protocol.PluginTerminalBinding{Source: extension.TerminalBindingValue, Value: `"project"`}}}
	declared := protocol.PluginTerminalView{
		ID: "memory", Title: "Agent memory", Contribution: "memory-provider",
		List: protocol.PluginTerminalList{
			Method: "memory/list", Rows: "Rows", Key: "ID", Paging: &protocol.PluginTerminalPaging{Input: "Cursor", Output: "NextCursor"},
			Input: []protocol.PluginTerminalInput{
				{Path: "CWD", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingContext, Context: extension.TerminalContextWorkspace}},
				{Path: "Scope", Value: scope},
			},
		},
		Actions: []protocol.PluginTerminalAction{{
			ID: "pin", Title: "Pin", Method: "memory/update", Target: extension.TerminalActionRow,
			Input: []protocol.PluginTerminalInput{
				{Path: "ID", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingRow, Field: "ID"}},
				{Path: "Revision", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingRow, Field: "Revision"}},
				{Path: "Pinned", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingValue, Value: `true`}},
				{Path: "Model/Provider", Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingForm, Form: &protocol.PluginTerminalFormField{Title: "Provider", Kind: extension.TerminalFormText, Default: &protocol.PluginTerminalBinding{Source: extension.TerminalBindingRow, Field: "Model/provider"}}}},
			},
		}},
	}
	views, problems := Views([]protocol.PluginInstallation{
		{ID: "installation", Enabled: true, Presentation: protocol.PluginPresentationAdmitted, Selected: protocol.PluginRelease{Name: "flame.memory", Digest: "digest", TerminalViews: []protocol.PluginTerminalView{declared}, Contributions: []protocol.PluginContribution{{ID: "memory-provider", Contract: string(contract.Identity().JSON())}}}},
		{ID: "disabled", Selected: protocol.PluginRelease{TerminalViews: []protocol.PluginTerminalView{declared}}},
	})
	if len(problems) != 0 || len(views) != 1 {
		t.Fatalf("views = %+v, problems = %v", views, problems)
	}
	return views[0]
}

func decodedInput(t *testing.T, request protocol.CallPluginRequest) map[string]any {
	t.Helper()
	var input map[string]any
	if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestListRequestBindsContextFormAndCursor(t *testing.T) {
	view := testView(t)
	form, err := view.ListForm(Context{})
	if err != nil || len(form) != 1 || form[0].Initial != "project" || form[0].Kind != extension.TerminalFormChoice {
		t.Fatalf("list form = %+v, %v", form, err)
	}
	request, err := view.ListRequest(Context{Workspace: "/work"}, map[string]string{"Scope": "user"}, "next")
	if err != nil {
		t.Fatal(err)
	}
	input := decodedInput(t, request)
	if input["CWD"] != "/work" || input["Scope"] != "user" || input["Cursor"] != "next" {
		t.Fatalf("list input = %v", input)
	}
	if request.InstallationID != "installation" || request.Target.Reference != "installation" || request.Method != "memory/list" {
		t.Fatalf("list request = %+v", request)
	}
	if _, err := view.ListRequest(Context{Workspace: "/work"}, map[string]string{"Scope": "team"}, ""); err == nil {
		t.Fatal("a scope outside the choice was accepted")
	}
}

func TestActionRequestReadsTheRowAndNestsPaths(t *testing.T) {
	view := testView(t)
	action, _ := view.Action("pin")
	row := Row{"ID": jsontext.Value(`"mem_1"`), "Revision": jsontext.Value(`3`), "Model": jsontext.Value(`{"provider":"openai"}`)}
	form, err := view.ActionForm(action, Context{}, row)
	if err != nil || len(form) != 1 || form[0].Initial != "openai" {
		t.Fatalf("action form = %+v, %v", form, err)
	}
	request, err := view.ActionRequest(action, Context{}, row, map[string]string{"Model/Provider": "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	input := decodedInput(t, request)
	model, _ := input["Model"].(map[string]any)
	if input["ID"] != "mem_1" || input["Revision"] != float64(3) || input["Pinned"] != true || model["Provider"] != "anthropic" {
		t.Fatalf("action input = %v", input)
	}
	if !view.Mutates(action) {
		t.Fatal("an update was not staged as a mutation")
	}
	if _, err := view.ActionRequest(action, Context{}, nil, nil); err == nil {
		t.Fatal("a row action ran without a row")
	}
}

func TestPageDecodesRowsAndCursor(t *testing.T) {
	view := testView(t)
	page, err := view.Page(&protocol.CallPluginResponse{Output: `{"Rows":[{"ID":"mem_1","Content":"fact"}],"NextCursor":"c2"}`})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Text("Content") != "fact" || page.Next != "c2" {
		t.Fatalf("page = %+v", page)
	}
}

func TestPageReadsNestedDataAndRejectsAnInvalidCursor(t *testing.T) {
	view := testView(t)
	view.List.Rows = "page/items"
	view.List.Paging.Output = "page/next"
	page, err := view.Page(&protocol.CallPluginResponse{Output: `{"page":{"items":[{"ID":"selected"}],"next":"continue"}}`})
	if err != nil || len(page.Rows) != 1 || page.Rows[0].Text("ID") != "selected" || page.Next != "continue" {
		t.Fatalf("page = %+v, err = %v", page, err)
	}
	if _, err := view.Page(&protocol.CallPluginResponse{Output: `{"page":{"items":[],"next":3}}`}); err == nil {
		t.Fatal("non-string cursor ended pagination silently")
	}
}
