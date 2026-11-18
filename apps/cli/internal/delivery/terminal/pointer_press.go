package terminal

type pressPhase uint8

const (
	pressIdle pressPhase = iota
	pressHeld
	pressDragged
)

// pointerPress follows one left-button press from down to up. A dragged press
// can no longer click, so selecting or scrolling never activates whatever lies
// under the release. Each view decides which motion counts as a drag.
type pointerPress[T comparable] struct {
	target T
	phase  pressPhase
}

func (p *pointerPress[T]) begin(target T) {
	*p = pointerPress[T]{target: target, phase: pressHeld}
}

// pressed reports the target of the press in progress, dragged or not.
func (p *pointerPress[T]) pressed() (T, bool) { return p.target, p.phase != pressIdle }

func (p *pointerPress[T]) drag() bool {
	if p.phase == pressIdle {
		return false
	}
	p.phase = pressDragged
	return true
}

// release ends the press and reports its target and whether it was a click.
func (p *pointerPress[T]) release() (target T, click bool) {
	target, click = p.target, p.phase == pressHeld
	p.cancel()
	return target, click
}

func (p *pointerPress[T]) cancel() { *p = pointerPress[T]{} }
