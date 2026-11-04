package toolset

import (
	"github.com/Tangerg/scope/core/content"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func appendToolOutputText(output toolcontract.Output, suffix string) toolcontract.Output {
	if suffix == "" {
		return output
	}
	augmented := output.Clone()
	if len(augmented.Content) == 0 && len(augmented.Details) > 0 {
		augmented.Content = append(augmented.Content, content.Block{Kind: content.Text, Text: string(augmented.Details) + suffix})
		return augmented
	}
	augmented.Content = append(augmented.Content, content.Block{Kind: content.Text, Text: suffix})
	return augmented
}
