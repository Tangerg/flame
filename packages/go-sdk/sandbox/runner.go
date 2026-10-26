package sandbox

import (
	"bytes"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
)

// A per-stream cap prevents isolated child output from exhausting host memory.
const MaxCommandOutputBytes = 256 << 10

// outputWithMarker tells the model how much of a command's output it is not
// seeing, which the bounded writer counts but deliberately does not phrase.
func outputWithMarker(output *capture.Writer) []byte {
	captured := bytes.Clone(output.Bytes())
	if dropped := output.Dropped(); dropped > 0 {
		return fmt.Appendf(captured, "\n... [%d bytes truncated] ...\n", dropped)
	}
	return captured
}
