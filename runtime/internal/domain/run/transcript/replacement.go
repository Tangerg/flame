package transcript

type Replacement struct {
	expected   Item
	state      Item
	toolResult *ToolResultBlob
}

func (i Item) replacedBy(state Item, err error) (Replacement, error) {
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: i, state: state}, nil
}

func (r Replacement) Expected() Item { return r.expected }

func (r Replacement) State() Item { return r.state }

func (r Replacement) ToolResult() *ToolResultBlob {
	if r.toolResult == nil {
		return nil
	}
	return new(*r.toolResult)
}
