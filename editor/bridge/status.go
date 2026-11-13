package bridge

// statusMsg answers "status": Tick counts the bridge's ticks (the one that
// handled the request included), Play and Paused are the mode, Scenes the
// names of the engine's scene stack, bottom first. The answer comes at the
// start of the tick that handled the request, in order with the answers to
// the messages before it, so a host also uses it as a round trip.
type statusMsg struct {
	Type   string   `json:"type"` // "status"
	Tick   uint64   `json:"tick"`
	Play   bool     `json:"play"`
	Paused bool     `json:"paused"`
	Scenes []string `json:"scenes"`
}

// status answers a "status" message.
func (b *Bridge) status() {
	scenes := []string{}
	for _, sc := range b.Engine().Scenes() {
		scenes = append(scenes, sc.Name())
	}
	b.send(statusMsg{Type: "status", Tick: b.ticks, Play: b.Playing(), Paused: b.Paused(), Scenes: scenes})
}
