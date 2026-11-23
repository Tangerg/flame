package sandbox

import (
	"bytes"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
)

const MaxCommandOutputBytes = 256 << 10

func outputWithMarker(output *capture.Writer) []byte {
	captured := bytes.Clone(output.Bytes())
	if dropped := output.Dropped(); dropped > 0 {
		return fmt.Appendf(captured, "\n... [%d bytes truncated] ...\n", dropped)
	}
	return captured
}
