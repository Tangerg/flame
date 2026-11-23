package execution

import (
	"fmt"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
)

func renderToolResultPreview(body, id string, previewBytes int) string {
	marker := fmt.Sprintf(
		"\n\n…[%d bytes offloaded to keep the context small — retrieve omitted bytes with %s: {\"result_id\":\"%s\"}; page with offset_bytes and limit_bytes.]…\n\n",
		len(body), toolResultReaderName, id,
	)
	return toolResultPreview(body, marker, previewBytes)
}

func renderToolResultRangePreview(body, id string, offsetBytes, previewBytes int) string {
	marker := fmt.Sprintf(
		"\n\n…[This content block spans %d bytes at blob offset %d. Retrieve with %s: {\"result_id\":\"%s\",\"offset_bytes\":%d}; page with limit_bytes within this block's range.]…\n\n",
		len(body), offsetBytes, toolResultReaderName, id, offsetBytes,
	)
	return toolResultPreview(body, marker, previewBytes)
}

func toolResultPreview(body, marker string, previewBytes int) string {
	head, tailStart := modeladapter.HeadTailCut(body, previewBytes)
	return body[:head] + marker + body[tailStart:]
}
