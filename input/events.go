package input

// Events are what happened during one [Router.Update]. The slices are reset
// (length 0, capacity kept) at the start of every Update, so the pointer
// from [Router.Events] is stable and the contents are valid until the next
// Update.
type Events struct {
	// DevicesConnected and DevicesDisconnected list pad hot-plugs.
	DevicesConnected    []DeviceInfo
	DevicesDisconnected []DeviceInfo
	// PlayersJoined and PlayersLeft list player indices (Join policy and
	// Player.Leave).
	PlayersJoined []int
	PlayersLeft   []int
	// DevicesLost lists devices that disconnected while assigned to a
	// player; the player keeps its slot. DevicesRestored lists lost pads
	// that came back (same GUID and name) and were reattached.
	DevicesLost     []PlayerDevice
	DevicesRestored []PlayerDevice
	// ActiveDeviceChanged lists players whose last-used device changed:
	// time to switch prompt glyphs.
	ActiveDeviceChanged []int
	// Captured holds the results of rebinding listens (see Player.Listen).
	Captured []Capture
}

// PlayerDevice pairs a player index with a device.
type PlayerDevice struct {
	Player int
	Device DeviceInfo
}

func (e *Events) reset() {
	e.DevicesConnected = e.DevicesConnected[:0]
	e.DevicesDisconnected = e.DevicesDisconnected[:0]
	e.PlayersJoined = e.PlayersJoined[:0]
	e.PlayersLeft = e.PlayersLeft[:0]
	e.DevicesLost = e.DevicesLost[:0]
	e.DevicesRestored = e.DevicesRestored[:0]
	e.ActiveDeviceChanged = e.ActiveDeviceChanged[:0]
	e.Captured = e.Captured[:0]
}
