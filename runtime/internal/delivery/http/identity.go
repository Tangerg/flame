package http

import (
	"runtime/debug"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func ServerInfoOrDefault() protocol.ServerInfo {
	loadOnce.Do(loadServerInfo)
	return loaded
}

var (
	loadOnce sync.Once
	loaded   protocol.ServerInfo
)

func loadServerInfo() {
	loaded = protocol.ServerInfo{Name: identity.ProductName, Version: "dev"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		loaded.Version = info.Main.Version
	}
}
