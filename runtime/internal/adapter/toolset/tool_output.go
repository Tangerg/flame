package toolset

import "github.com/Tangerg/scope/core/chat"

func appendToolOutputText(output chat.ToolOutput, suffix string) chat.ToolOutput {
	if suffix == "" {
		return output
	}
	augmented := output.Clone()
	if len(augmented.Content) == 0 && len(augmented.Details) > 0 {
		augmented.Content = append(augmented.Content, chat.ToolContent{Kind: chat.PartText, Text: string(augmented.Details) + suffix})
		return augmented
	}
	augmented.Content = append(augmented.Content, chat.ToolContent{Kind: chat.PartText, Text: suffix})
	return augmented
}
