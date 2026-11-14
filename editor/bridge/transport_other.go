//go:build !(js && wasm)

package bridge

import (
	"log"
	"os"

	"github.com/weakpixel/engine/editor"
)

// ConnectHost returns a bridge to the host named by the WEAKPIXEL_BRIDGE
// environment variable (host:port, with the secret in WEAKPIXEL_BRIDGE_TOKEN;
// see [DialHost]), or false when the variable is not set. A game started by
// an MCP server or another native host runs this way. A host that cannot be
// reached is logged and also gives false.
func ConnectHost(cfg editor.Config) (*Bridge, bool) {
	addr := os.Getenv(EnvBridge)
	if addr == "" {
		return nil, false
	}
	b, err := DialHost(addr, os.Getenv(EnvBridgeToken), cfg)
	if err != nil {
		log.Print(err)
		return nil, false
	}
	return b, true
}
