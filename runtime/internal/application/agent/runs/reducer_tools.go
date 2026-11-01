package runs

import (
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func newToolInvocation(source tool.Ref, name string, arguments tool.Arguments, result *tool.Result) *transcript.ToolInvocation {
	return &transcript.ToolInvocation{
		Name:   name,
		Source: source,
		Input:  transcript.AdmittedInput(arguments),
		Result: result,
	}
}

func parseToolArguments(raw string) (tool.Arguments, error) {
	return tool.ParseArguments(raw)
}
