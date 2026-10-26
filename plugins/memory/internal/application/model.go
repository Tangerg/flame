package coordination

import "errors"

const (
	OperationExtract = "memory.extract"
	OperationCurate  = "memory.curate"
)

type Prompt struct {
	Operation, SystemPrompt, UserPrompt string
	MaxInputBytes                       int
	MaxOutputTokens                     int64
}

const MaximumModelCalls = 2

const MaximumModelInputBytes = 512 * 1024
const MaximumModelOutputTokens int64 = 2048

func (p Prompt) Validate() error {
	if p.Operation != OperationExtract && p.Operation != OperationCurate || p.MaxInputBytes <= 0 || p.MaxInputBytes > MaximumModelInputBytes || p.MaxOutputTokens <= 0 || p.MaxOutputTokens > MaximumModelOutputTokens || len(p.SystemPrompt)+len(p.UserPrompt) > p.MaxInputBytes {
		return errors.New("memory: completion prompt refused")
	}
	return nil
}
