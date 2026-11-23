package terminal

import "time"

const confirmationWindow = 800 * time.Millisecond

type confirmationKind uint8

const (
	confirmClearDraft confirmationKind = iota + 1
	confirmQuit
)

type pressConfirmation struct {
	kind     confirmationKind
	deadline time.Time
}

func (p *pressConfirmation) Confirm(kind confirmationKind, now time.Time) bool {
	if p.kind == kind && !now.After(p.deadline) {
		p.Reset()
		return true
	}
	p.kind = kind
	p.deadline = now.Add(confirmationWindow)
	return false
}

func (p *pressConfirmation) Armed(kind confirmationKind) bool {
	return p.kind == kind
}

func (p *pressConfirmation) Reset() {
	*p = pressConfirmation{}
}
