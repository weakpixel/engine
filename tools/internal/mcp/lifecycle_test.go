package mcp

import (
	"slices"
	"testing"
)

func TestLifecycle(t *testing.T) {
	t.Run("modern discover", func(t *testing.T) {
		c := startServer(t, NewServer(Info{"weakpixel", "1"}, "use it"))
		m := c.call(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{` + modern + `}}`)
		r := m["result"].(map[string]any)
		if r["resultType"] != "complete" || r["cacheScope"] != "private" || r["ttlMs"] == nil {
			t.Errorf("result fields: %v", r)
		}
		if got := r["supportedVersions"].([]any); len(got) != 2 || got[0] != VersionModern || got[1] != VersionLegacy {
			t.Errorf("supportedVersions = %v", got)
		}
		info := r["_meta"].(map[string]any)["io.modelcontextprotocol/serverInfo"].(map[string]any)
		if info["name"] != "weakpixel" {
			t.Errorf("serverInfo = %v", info)
		}
		if r["instructions"] != "use it" {
			t.Errorf("instructions = %v", r["instructions"])
		}
		if _, ok := r["capabilities"].(map[string]any)["tools"]; !ok {
			t.Errorf("capabilities = %v", r["capabilities"])
		}
		// Every modern request is self-contained: tools/list needs no handshake.
		m = c.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{` + modern + `}}`)
		if m["result"].(map[string]any)["resultType"] != "complete" {
			t.Errorf("tools/list: %v", m)
		}
	})

	t.Run("unsupported version", func(t *testing.T) {
		c := startServer(t, NewServer(Info{"weakpixel", "1"}, ""))
		m := c.call(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01"}}}`)
		if errCode(m) != CodeUnsupportedProtocolVersion {
			t.Fatalf("got %v", m)
		}
		data := m["error"].(map[string]any)["data"].(map[string]any)
		if data["requested"] != "1900-01-01" || len(data["supported"].([]any)) != 2 {
			t.Errorf("data = %v", data)
		}
		// A legacy version without initialize is not served either.
		m = c.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25"}}}`)
		if errCode(m) != CodeUnsupportedProtocolVersion {
			t.Errorf("legacy version without initialize: %v", m)
		}
	})

	for _, tc := range []struct{ asked, want string }{
		{"2025-11-25", "2025-11-25"},
		{"2025-06-18", "2025-06-18"},
		{"2024-11-05", "2025-11-25"},
	} {
		t.Run("initialize "+tc.asked, func(t *testing.T) {
			c := startServer(t, NewServer(Info{"weakpixel", "1"}, ""))
			before := c.call(`{"jsonrpc":"2.0","id":0,"method":"tools/list"}`)
			if errCode(before) != CodeInvalidParams {
				t.Errorf("tools/list before initialize: %v", before)
			}
			m := c.call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + tc.asked + `","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`)
			r := m["result"].(map[string]any)
			if r["protocolVersion"] != tc.want {
				t.Errorf("protocolVersion = %v, want %s", r["protocolVersion"], tc.want)
			}
			if r["serverInfo"].(map[string]any)["name"] != "weakpixel" {
				t.Errorf("serverInfo = %v", r["serverInfo"])
			}
			c.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
			if m := c.call(`{"jsonrpc":"2.0","id":2,"method":"ping"}`); errCode(m) != 0 {
				t.Errorf("ping: %v", m)
			}
			m = c.call(`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)
			if _, ok := m["result"].(map[string]any)["tools"]; !ok {
				t.Errorf("tools/list after initialize: %v", m)
			}
		})
	}

	if !slices.Equal(SupportedVersions, []string{"2026-07-28", "2025-11-25"}) {
		t.Errorf("SupportedVersions = %v", SupportedVersions)
	}
}
