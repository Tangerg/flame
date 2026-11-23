package terminal

type pressPhase uint8

const (
	pressIdle pressPhase = iota
	pressHeld
	pressDragged
)

type pointerPress[T comparable] struct {
	target T
	phase  pressPhase
}

func (p *pointerPress[T]) begin(target T) {
	*p = pointerPress[T]{target: target, phase: pressHeld}
}

func (p *pointerPress[T]) pressed() (T, bool) { return p.target, p.phase != pressIdle }

func (p *pointerPress[T]) drag() bool {
	if p.phase == pressIdle {
		return false
	}
	p.phase = pressDragged
	return true
}

func (p *pointerPress[T]) release() (target T, click bool) {
	target, click = p.target, p.phase == pressHeld
	p.cancel()
	return target, click
}

func (p *pointerPress[T]) cancel() { *p = pointerPress[T]{} }
