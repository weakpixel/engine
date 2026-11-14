//go:build editor

// Command fakegame is a stand-in for a game's editor build in the tests of
// the MCP server: it speaks the editor protocol over the TCP connection the
// server names in the environment, without an engine. It prints a line to
// stdout and to stderr, answers the messages the tools send, and ends when
// the connection closes, as a bridge does. A document whose text contains
// REJECT is refused.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// onePixelPNG is a valid 1x1 PNG.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func main() {
	fmt.Println("fakegame stdout hello")
	fmt.Fprintln(os.Stderr, "fakegame stderr hello")
	fmt.Printf("stage=%s path=%s headless=%q\n", os.Getenv("WEAKPIXEL_STAGE"), os.Getenv("WEAKPIXEL_PATH"), os.Getenv("WEAKPIXEL_HEADLESS"))
	conn, err := net.Dial("tcp", os.Getenv("WEAKPIXEL_BRIDGE"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprintln(conn, os.Getenv("WEAKPIXEL_BRIDGE_TOKEN"))
	send := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(conn, "%s\n", data)
	}
	send(map[string]any{"type": "ready", "version": 0})
	r := bufio.NewReaderSize(conn, 1<<20)
	play := false
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			fmt.Println("fakegame: connection closed")
			return
		}
		var m map[string]any
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		switch m["type"] {
		case "document", "open":
			if text, _ := m["text"].(string); strings.Contains(text, "REJECT") {
				send(map[string]any{"type": "applied", "error": `object "x": component demo:Health: rejected by the test game`})
				continue
			}
			send(map[string]any{"type": "objects", "objects": []any{map[string]any{"id": "a", "group": "demo", "class": "Crate"}}})
			send(map[string]any{"type": "applied", "error": ""})
		case "play":
			play = true
			send(map[string]any{"type": "mode", "play": true})
		case "stop":
			if play {
				play = false
				send(map[string]any{"type": "mode", "play": false})
			}
		case "screenshot":
			png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
			send(map[string]any{"type": "screenshot", "id": m["id"], "png": png, "width": 1, "height": 1, "tick": 7})
		case "step", "select", "camera", "frame", "settings", "assetsChanged", "mapLayerHidden":
			if m["type"] == "assetsChanged" {
				fmt.Println("assetsChanged", m["paths"])
			}
		default:
			send(map[string]any{"type": "error", "message": fmt.Sprintf("bridge: unknown message type %q", m["type"])})
		}
	}
}
