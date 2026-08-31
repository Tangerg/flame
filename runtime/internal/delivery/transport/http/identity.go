package http

import (
	"runtime/debug"
	"sync"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ServerInfoOrDefault returns a ServerInfo populated from the
// build-info recorded by the Go toolchain (module version, commit
// hash via VCS info). Caller can override the result before passing
// to NewServer if they want a custom identity.
func ServerInfoOrDefault() protocol.ServerInfo {
	loadOnce.Do(loadServerInfo)
	return loaded
}

var (
	loadOnce sync.Once
	loaded   protocol.ServerInfo
)

func loadServerInfo() {
	loaded = protocol.ServerInfo{Name: runtimeidentity.ProductName, Version: "dev"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		loaded.Version = info.Main.Version
	}
}
