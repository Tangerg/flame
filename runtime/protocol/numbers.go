package protocol

import (
	"time"

	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// MaximumExactJSONInteger is the largest integer-valued identity Flame places
// on its public JSON wire. Staying inside this shared envelope keeps revision,
// sequence, and optimistic-concurrency comparisons exact in JavaScript clients.
const MaximumExactJSONInteger = exactint.Maximum

// MaximumDurationMilliseconds is the largest millisecond count that can be
// projected to time.Duration without overflow.
const MaximumDurationMilliseconds = int64((1<<63)-1) / int64(time.Millisecond)

// MaximumDurationSeconds is the largest whole-second count that can be
// projected to time.Duration and later encoded again without overflow.
const MaximumDurationSeconds = int64((1<<63)-1) / int64(time.Second)
