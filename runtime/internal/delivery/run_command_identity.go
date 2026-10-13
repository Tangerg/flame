package delivery

import (
	"cmp"
	"slices"

	"github.com/Tangerg/flame/runtime/protocol"
)

func runCommandFingerprint(name Name, parameters any, capabilities *protocol.ClientCapabilities) (string, error) {
	if request, ok := parameters.(protocol.ResumeRunRequest); ok {
		request.Responses = slices.Clone(request.Responses)
		slices.SortFunc(request.Responses, func(a, b protocol.InterruptResponse) int { return cmp.Compare(a.ItemID, b.ItemID) })
		parameters = request
	}
	identity := struct {
		Request        any                      `json:"request"`
		InterruptTypes []protocol.InterruptType `json:"interruptTypes,omitempty"`
		ChildRuns      bool                     `json:"childRuns"`
	}{Request: parameters}
	if capabilities != nil {
		identity.InterruptTypes = slices.Clone(capabilities.InterruptTypes)
		slices.Sort(identity.InterruptTypes)
		identity.ChildRuns = capabilities.Features[protocol.FeatureSubagents].Enabled
	}
	return operationFingerprint(name, identity)
}
