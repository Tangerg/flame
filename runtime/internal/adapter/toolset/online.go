package toolset

import (
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/httpreq"
)

type OnlineConfig struct {
	HTTPAllowedHosts []string
}

func buildOnline(online OnlineConfig) ([]toolcontract.Tool, error) {
	if len(online.HTTPAllowedHosts) == 0 {
		return nil, nil
	}
	client, err := httpreq.NewClient(httpreq.ClientConfig{AllowedHosts: online.HTTPAllowedHosts})
	if err != nil {
		return nil, err
	}
	executable, err := httpreq.NewTool(client)
	if err != nil {
		return nil, err
	}
	return []toolcontract.Tool{executable}, nil
}
