package extension

import "strings"

const ModelProviderContract = "flame.model-provider.v1"

func ModelProviderRole(provider string) string { return ModelProviderContract + "/" + provider }
func ProviderFromRole(role string) (string, bool) {
	provider, found := strings.CutPrefix(role, ModelProviderContract+"/")
	return provider, found && provider != ""
}
