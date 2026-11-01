package plugin

import (
	"encoding/json/jsontext"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/qimu/portable"
)

func terminalViewRelease(t *testing.T, views ...TerminalView) (Release, error) {
	t.Helper()
	listInput := `{"type":"object","required":["Scope","CWD"],"properties":{"Scope":{"allOf":[{"enum":["project","user"]}],"type":"string"},"CWD":{"type":"string"},"Cursor":{"type":"string"}}}`
	listOutput := `{"type":"object","properties":{"Rows":{"type":"array","items":{"$ref":"#/$defs/memory.Item"}},"NextCursor":{"type":"string"}},"$defs":{"memory.Item":{"type":"object","properties":{"ID":{"type":"string"},"Content":{"type":"string"},"Revision":{"type":"integer"},"Pinned":{"type":"boolean"}}}}}`
	updateInput := `{"type":"object","required":["ID","ExpectedRevision","Content","Pinned"],"properties":{"ID":{"type":"string"},"ExpectedRevision":{"type":"integer"},"Content":{"type":"string"},"Pinned":{"type":"boolean"},"Model":{"type":"object","required":["Provider"],"properties":{"Provider":{"type":"string"},"Model":{"type":"string"}}}}}`
	contract, err := portable.NewCapability(
		portable.CapabilityConfig{Reference: "memory", Composition: portable.Selection, Scope: extension.ScopeInstallation},
		portable.MethodDescriptor{Name: "memory/list", Kind: portable.Query, Target: extension.ScopeInstallation, Input: jsontext.Value(listInput), Output: jsontext.Value(listOutput)},
		portable.MethodDescriptor{Name: "memory/update", Kind: portable.Mutation, Target: extension.ScopeInstallation, Input: jsontext.Value(updateInput), Output: jsontext.Value(`{"type":"object"}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	return NewRelease(testDigest("terminal"), Declaration{
		Name:          "terminal",
		Servers:       []Server{{Name: serverName("backend"), Startup: DeclaredConnection{Connection: mcpserver.StdioConnection{Command: "backend"}}}},
		Contributions: []Contribution{{ID: "memory-provider", Server: serverName("backend"), Export: "memory", Contract: contract.Identity()}},
		TerminalViews: views,
	})
}

func literal(t *testing.T, value any) jsonvalue.Value {
	t.Helper()
	encoded, err := jsonvalue.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func memoryView(t *testing.T) TerminalView {
	return TerminalView{
		ID: "memory", Title: "Agent memory", Contribution: "memory-provider",
		List: TerminalList{
			Method: "memory/list", Rows: "Rows", Key: "ID",
			Paging:  &TerminalPaging{Input: "Cursor", Output: "NextCursor"},
			Columns: []TerminalColumn{{Title: "Content", Field: "Content"}},
			Input: []InputBinding{
				{Path: "CWD", Value: Binding{Source: extension.TerminalBindingContext, Context: extension.TerminalContextWorkspace}},
				{Path: "Scope", Value: Binding{Source: extension.TerminalBindingForm, Form: &FormField{Title: "Scope", Default: &Binding{Source: extension.TerminalBindingValue, Literal: literal(t, "project")}}}},
			},
		},
		Actions: []TerminalAction{{
			ID: "edit", Title: "Edit", Method: "memory/update", Target: extension.TerminalActionRow,
			Input: []InputBinding{
				{Path: "ID", Value: Binding{Source: extension.TerminalBindingRow, Field: "ID"}},
				{Path: "ExpectedRevision", Value: Binding{Source: extension.TerminalBindingRow, Field: "Revision"}},
				{Path: "Content", Value: Binding{Source: extension.TerminalBindingForm, Form: &FormField{Title: "Content", Default: &Binding{Source: extension.TerminalBindingRow, Field: "Content"}}}},
				{Path: "Pinned", Value: Binding{Source: extension.TerminalBindingRow, Field: "Pinned"}},
				{Path: "Model/Provider", Value: Binding{Source: extension.TerminalBindingForm, Form: &FormField{Title: "Provider"}}},
			},
		}},
	}
}

func TestTerminalViewsResolveFormKindsFromTheContract(t *testing.T) {
	release, err := terminalViewRelease(t, memoryView(t))
	if err != nil {
		t.Fatal(err)
	}
	views := release.TerminalViews()
	if len(views) != 1 {
		t.Fatalf("views = %+v", views)
	}
	scope := views[0].List.Input[1].Value.Form
	if scope.Kind != extension.TerminalFormChoice || !slices.Equal(scope.Options, []string{"project", "user"}) {
		t.Fatalf("scope form = %+v, want a project/user choice", scope)
	}
	content := views[0].Actions[0].Input[2].Value.Form
	if content.Kind != extension.TerminalFormText {
		t.Fatalf("content form = %+v, want text", content)
	}
}

func TestTerminalViewsRefuseBindingsTheContractCannotHonor(t *testing.T) {
	for name, change := range map[string]func(*TerminalView){

		"overlapping input paths": func(v *TerminalView) {
			v.Actions[0].Input = append(v.Actions[0].Input, InputBinding{Path: "Model", Value: Binding{Source: extension.TerminalBindingValue, Literal: literal(t, map[string]string{"Provider": "x"})}})
		},
		"cursor output is not a string": func(v *TerminalView) { v.List.Paging.Output = "Rows" },
		"unknown contribution":          func(v *TerminalView) { v.Contribution = "missing" },
		"list is a command":             func(v *TerminalView) { v.List.Method = "memory/update" },
		"unbound required list input": func(v *TerminalView) {
			v.List.Input = v.List.Input[:1]
		},
		"rows are not an array": func(v *TerminalView) { v.List.Rows = "NextCursor" },
		"unknown column":        func(v *TerminalView) { v.List.Columns = []TerminalColumn{{Title: "Title", Field: "Title"}} },
		"list reads a row": func(v *TerminalView) {
			v.List.Input[0].Value = Binding{Source: extension.TerminalBindingRow, Field: "ID"}
		},
		"view action reads a row": func(v *TerminalView) { v.Actions[0].Target = extension.TerminalActionView },
		"unknown input property":  func(v *TerminalView) { v.Actions[0].Input[0].Path = "Identity" },
		"unknown row field":       func(v *TerminalView) { v.Actions[0].Input[0].Value.Field = "Title" },
		"partially bound object":  func(v *TerminalView) { v.Actions[0].Input[4].Path = "Model/Model" },
		"unbound required action input": func(v *TerminalView) {
			v.Actions[0].Input = v.Actions[0].Input[1:]
		},
		"duplicate action": func(v *TerminalView) { v.Actions = append(v.Actions, v.Actions[0]) },
		"object form field": func(v *TerminalView) {
			v.Actions[0].Input[4] = InputBinding{Path: "Model", Value: Binding{Source: extension.TerminalBindingForm, Form: &FormField{Title: "Model"}}}
		},
		"unknown context": func(v *TerminalView) { v.List.Input[0].Value.Context = "home" },
	} {
		t.Run(name, func(t *testing.T) {
			view := memoryView(t)
			change(&view)
			if _, err := terminalViewRelease(t, view); !errors.Is(err, ErrInvalid) {
				t.Fatalf("release error = %v, want an invalid declaration", err)
			}
		})
	}
}
