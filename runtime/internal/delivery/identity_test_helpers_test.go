package delivery

import "github.com/Tangerg/flame/runtime/protocol"

func wireUserServer(name string) protocol.MCPServerID {
	return protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: name}
}
