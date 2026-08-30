package protocol

import "github.com/Tangerg/flame/runtime/internal/exactint"

// MaximumExactJSONInteger is the largest integer-valued identity Flame places
// on its public JSON wire. Staying inside this shared envelope keeps revision,
// sequence, and optimistic-concurrency comparisons exact in JavaScript clients.
const MaximumExactJSONInteger = exactint.Maximum
