package extension

import "slices"

// TerminalActionTarget is what a terminal view action acts on: the whole view
// or one selected row.
type TerminalActionTarget string

const (
	TerminalActionView TerminalActionTarget = "view"
	TerminalActionRow  TerminalActionTarget = "row"
)

func (TerminalActionTarget) Values() []string {
	return []string{string(TerminalActionView), string(TerminalActionRow)}
}

func (t TerminalActionTarget) Valid() bool { return slices.Contains(t.Values(), string(t)) }

// TerminalBindingSource is where a terminal view takes one input value from.
type TerminalBindingSource string

const (
	TerminalBindingValue   TerminalBindingSource = "value"
	TerminalBindingRow     TerminalBindingSource = "row"
	TerminalBindingContext TerminalBindingSource = "context"
	TerminalBindingForm    TerminalBindingSource = "form"
)

func (TerminalBindingSource) Values() []string {
	return []string{string(TerminalBindingValue), string(TerminalBindingRow), string(TerminalBindingContext), string(TerminalBindingForm)}
}

func (s TerminalBindingSource) Valid() bool { return slices.Contains(s.Values(), string(s)) }

// TerminalContext is a fact a terminal client supplies about where it runs.
type TerminalContext string

const (
	TerminalContextWorkspace TerminalContext = "workspace"
	TerminalContextTimeZone  TerminalContext = "timeZone"
)

func (TerminalContext) Values() []string {
	return []string{string(TerminalContextWorkspace), string(TerminalContextTimeZone)}
}

func (c TerminalContext) Valid() bool { return slices.Contains(c.Values(), string(c)) }

// TerminalFormKind is how a person enters one form field.
type TerminalFormKind string

const (
	TerminalFormText    TerminalFormKind = "text"
	TerminalFormInteger TerminalFormKind = "integer"
	TerminalFormNumber  TerminalFormKind = "number"
	TerminalFormBoolean TerminalFormKind = "boolean"
	TerminalFormChoice  TerminalFormKind = "choice"
)

func (TerminalFormKind) Values() []string {
	return []string{string(TerminalFormText), string(TerminalFormInteger), string(TerminalFormNumber), string(TerminalFormBoolean), string(TerminalFormChoice)}
}
