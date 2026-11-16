package coordination

import "errors"

type Prompt struct {
	SystemPrompt, UserPrompt string
	MaxOutputTokens          int64
}

const MaximumModelCalls = 2

const MaximumModelInputBytes = 512 * 1024
const MaximumModelOutputTokens int64 = 2048

func (p Prompt) Validate() error {
	if p.MaxOutputTokens <= 0 || p.MaxOutputTokens > MaximumModelOutputTokens || len(p.SystemPrompt)+len(p.UserPrompt) > MaximumModelInputBytes {
		return errors.New("memory: completion prompt refused")
	}
	return nil
}
