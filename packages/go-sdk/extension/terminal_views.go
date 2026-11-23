package extension

import "slices"

type TerminalActionTarget string

const (
	TerminalActionView TerminalActionTarget = "view"
	TerminalActionRow  TerminalActionTarget = "row"
)

func (TerminalActionTarget) Values() []string {
	return []string{string(TerminalActionView), string(TerminalActionRow)}
}

func (t TerminalActionTarget) Valid() bool { return slices.Contains(t.Values(), string(t)) }

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

type TerminalContext string

const (
	TerminalContextWorkspace TerminalContext = "workspace"
	TerminalContextTimeZone  TerminalContext = "timeZone"
)

func (TerminalContext) Values() []string {
	return []string{string(TerminalContextWorkspace), string(TerminalContextTimeZone)}
}

func (c TerminalContext) Valid() bool { return slices.Contains(c.Values(), string(c)) }

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
