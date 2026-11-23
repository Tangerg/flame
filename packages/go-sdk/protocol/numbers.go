package protocol

import (
	"math"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

const MaximumExactJSONInteger = exactint.Maximum

const MaximumDurationMilliseconds = math.MaxInt64 / int64(time.Millisecond)

const MaximumDurationSeconds = math.MaxInt64 / int64(time.Second)
