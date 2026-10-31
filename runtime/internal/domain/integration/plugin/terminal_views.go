package plugin

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/qimu/portable"
)

// TerminalView is a management surface a package contributes to terminal
// clients: a list read through one callable contribution and actions that call
// its methods. Every input value is bound in the declaration, so a client
// renders the view from the declaration and the contract alone, without
// compiling in the package's types.
type TerminalView struct {
	ID           string
	Title        string
	Contribution ContributionID
	List         TerminalList
	Actions      []TerminalAction
}

// TerminalList reads the view's rows. Rows names the output property that holds
// them and Key the row property that identifies one.
type TerminalList struct {
	Method  string
	Input   []InputBinding
	Rows    string
	Key     string
	Paging  *TerminalPaging
	Columns []TerminalColumn
}

// TerminalPaging pages a list: the output property carries the next cursor and
// the input property sends it back.
type TerminalPaging struct {
	Input  string
	Output string
}

type TerminalColumn struct {
	Title string
	Field string
}

// TerminalAction calls one method, on the view or on a selected row.
type TerminalAction struct {
	ID      string
	Title   string
	Method  string
	Target  extension.TerminalActionTarget
	Confirm bool
	Input   []InputBinding
}

// InputBinding supplies the input property at Path, a '/'-separated walk
// through nested objects.
type InputBinding struct {
	Path  string
	Value Binding
}

// Binding is where one input value comes from: a literal, a field of the
// selected row, a client context fact, or a form field the person fills. Exactly
// the member Source names is set.
type Binding struct {
	Source  extension.TerminalBindingSource
	Literal jsonvalue.Value
	Field   string
	Context extension.TerminalContext
	Form    *FormField
}

// FormField asks the person for a value, starting from Default when set. Kind
// and Options are read from the bound property's schema at admission, so a
// client renders the field without interpreting the contract.
type FormField struct {
	Title   string
	Default *Binding
	Kind    extension.TerminalFormKind
	Options []string
}

func (v TerminalView) clone() TerminalView {
	v.List.Input = cloneBindings(v.List.Input)
	if v.List.Paging != nil {
		paging := *v.List.Paging
		v.List.Paging = &paging
	}
	v.List.Columns = slices.Clone(v.List.Columns)
	v.Actions = slices.Clone(v.Actions)
	for i := range v.Actions {
		v.Actions[i].Input = cloneBindings(v.Actions[i].Input)
	}
	return v
}

func cloneBindings(bindings []InputBinding) []InputBinding {
	bindings = slices.Clone(bindings)
	for i := range bindings {
		bindings[i].Value = bindings[i].Value.clone()
	}
	return bindings
}

func (b Binding) clone() Binding {
	if b.Form != nil {
		form := *b.Form
		form.Options = slices.Clone(form.Options)
		if form.Default != nil {
			value := form.Default.clone()
			form.Default = &value
		}
		b.Form = &form
	}
	return b
}

func (b *Builder) AdmitTerminalView(value TerminalView) error {
	value = value.clone()
	if !identity.ValidDeclarationID(value.ID) {
		return refuse(DiagnosticInvalidDeclaration, "invalid terminal view identity %q", value.ID)
	}
	if !validTerminalTitle(value.Title) {
		return refuse(DiagnosticInvalidDeclaration, "invalid terminal view %q title", value.ID)
	}
	if slices.ContainsFunc(b.declaration.TerminalViews, func(current TerminalView) bool { return current.ID == value.ID }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate terminal view %q", value.ID)
	}
	contribution, found := b.release().Contribution(value.Contribution)
	if !found {
		return refuse(DiagnosticInvalidDependencies, "terminal view %q has no declared callable contribution", value.ID)
	}
	contract, err := portable.ParseCapability(contribution.Contract.JSON())
	if err != nil {
		return refuse(DiagnosticInvalidDependencies, "terminal view %q contract: %v", value.ID, err)
	}
	if err := admitTerminalList(contract, value.List); err != nil {
		return refuse(DiagnosticInvalidDeclaration, "terminal view %q list: %v", value.ID, err)
	}
	rows, err := listRowSchema(contract, value.List)
	if err != nil {
		return refuse(DiagnosticInvalidDeclaration, "terminal view %q list: %v", value.ID, err)
	}
	actions := make(map[string]bool, len(value.Actions))
	for _, action := range value.Actions {
		if actions[action.ID] {
			return refuse(DiagnosticInvalidDeclaration, "terminal view %q repeats action %q", value.ID, action.ID)
		}
		actions[action.ID] = true
		if err := admitTerminalAction(contract, rows, action); err != nil {
			return refuse(DiagnosticInvalidDeclaration, "terminal view %q action %q: %v", value.ID, action.ID, err)
		}
	}
	b.declaration.TerminalViews = append(b.declaration.TerminalViews, value)
	return nil
}

func validTerminalTitle(title string) bool {
	return title != "" && len(title) <= MaxPresentationTitleBytes && utf8.ValidString(title)
}

func admitTerminalList(contract *portable.Capability, list TerminalList) error {
	method, err := terminalMethod(contract, list.Method)
	if err != nil {
		return err
	}
	if method.Kind() != portable.Query {
		return fmt.Errorf("method %q is not a query", list.Method)
	}
	input, err := newSchemaNode(method.Descriptor().Input)
	if err != nil {
		return err
	}
	bound := make([]string, 0, len(list.Input)+1)
	if list.Paging != nil {
		output, err := newSchemaNode(method.Descriptor().Output)
		if err != nil {
			return err
		}
		if _, ok := input.property(list.Paging.Input); !ok {
			return fmt.Errorf("paging input %q is not an input property", list.Paging.Input)
		}
		if _, ok := output.property(list.Paging.Output); !ok {
			return fmt.Errorf("paging output %q is not an output property", list.Paging.Output)
		}
		bound = append(bound, list.Paging.Input)
	}
	for _, binding := range list.Input {
		if binding.Value.Source == extension.TerminalBindingRow {
			return fmt.Errorf("input %q reads a row before the list has one", binding.Path)
		}
	}
	return admitBindings(input, list.Input, nil, bound)
}

// listRowSchema resolves the schema of one row, which row bindings, the key and
// the columns read.
func listRowSchema(contract *portable.Capability, list TerminalList) (schemaNode, error) {
	method, _ := contract.Method(list.Method)
	output, err := newSchemaNode(method.Descriptor().Output)
	if err != nil {
		return schemaNode{}, err
	}
	rows, ok := output.property(list.Rows)
	if !ok {
		return schemaNode{}, fmt.Errorf("rows %q is not an output property", list.Rows)
	}
	row, ok := rows.items()
	if !ok {
		return schemaNode{}, fmt.Errorf("rows %q is not an array", list.Rows)
	}
	if _, ok := row.property(list.Key); !ok {
		return schemaNode{}, fmt.Errorf("key %q is not a row property", list.Key)
	}
	if len(list.Columns) == 0 {
		return schemaNode{}, fmt.Errorf("a list names at least one column")
	}
	for _, column := range list.Columns {
		if !validTerminalTitle(column.Title) {
			return schemaNode{}, fmt.Errorf("column %q has an invalid title", column.Field)
		}
		if _, ok := row.property(column.Field); !ok {
			return schemaNode{}, fmt.Errorf("column %q is not a row property", column.Field)
		}
	}
	return row, nil
}

func admitTerminalAction(contract *portable.Capability, row schemaNode, action TerminalAction) error {
	if !identity.ValidDeclarationID(action.ID) {
		return fmt.Errorf("invalid action identity")
	}
	if !validTerminalTitle(action.Title) {
		return fmt.Errorf("invalid action title")
	}
	if !action.Target.Valid() {
		return fmt.Errorf("unknown target %q", action.Target)
	}
	method, err := terminalMethod(contract, action.Method)
	if err != nil {
		return err
	}
	input, err := newSchemaNode(method.Descriptor().Input)
	if err != nil {
		return err
	}
	var rows *schemaNode
	if action.Target == extension.TerminalActionRow {
		rows = &row
	}
	return admitBindings(input, action.Input, rows, nil)
}

func terminalMethod(contract *portable.Capability, name string) (*portable.Method, error) {
	method, found := contract.Method(name)
	if !found {
		return nil, fmt.Errorf("method %q is not in the contribution contract", name)
	}
	return method, nil
}

// admitBindings checks that every binding names an input property, reads only
// what its target can supply, and that every required property on each bound
// object is bound. row is nil when no row is selected.
func admitBindings(input schemaNode, bindings []InputBinding, row *schemaNode, alsoBound []string) error {
	paths := slices.Clone(alsoBound)
	for _, binding := range bindings {
		if slices.Contains(paths, binding.Path) {
			return fmt.Errorf("input %q is bound twice", binding.Path)
		}
		property, ok := input.property(binding.Path)
		if !ok {
			return fmt.Errorf("input %q is not an input property", binding.Path)
		}
		if err := admitBinding(binding.Value, property, row, true); err != nil {
			return fmt.Errorf("input %q: %w", binding.Path, err)
		}
		paths = append(paths, binding.Path)
	}
	return input.requireBound("", paths)
}

func admitBinding(value Binding, property schemaNode, row *schemaNode, formAllowed bool) error {
	switch value.Source {
	case extension.TerminalBindingValue:
		if len(value.Literal.JSON()) == 0 {
			return fmt.Errorf("value binding has no value")
		}
	case extension.TerminalBindingRow:
		if row == nil {
			return fmt.Errorf("reads a row the target does not select")
		}
		if _, ok := row.property(value.Field); !ok {
			return fmt.Errorf("row field %q is not a row property", value.Field)
		}
	case extension.TerminalBindingContext:
		if !value.Context.Valid() {
			return fmt.Errorf("unknown context %q", value.Context)
		}
	case extension.TerminalBindingForm:
		if !formAllowed || value.Form == nil {
			return fmt.Errorf("a form field cannot appear here")
		}
		if !validTerminalTitle(value.Form.Title) {
			return fmt.Errorf("form field has an invalid title")
		}
		kind, options, ok := property.formKind()
		if !ok {
			return fmt.Errorf("a form field must be a string, number, integer, boolean or string enum")
		}
		value.Form.Kind, value.Form.Options = kind, options
		if value.Form.Default != nil {
			if err := admitBinding(*value.Form.Default, property, row, false); err != nil {
				return fmt.Errorf("default: %w", err)
			}
		}
	default:
		return fmt.Errorf("unknown binding source %q", value.Source)
	}
	return nil
}

// TerminalViews returns owned copies of the release's terminal views.
func (r Release) TerminalViews() []TerminalView {
	views := slices.Clone(r.declaration.TerminalViews)
	for i := range views {
		views[i] = views[i].clone()
	}
	return views
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}
