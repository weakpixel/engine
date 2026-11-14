package input

import (
	"slices"
)

// devSet is the set of devices whose controls a player reads this tick, with
// pads resolved to their index in the frame.
type devSet struct {
	kb, mouse, touch bool
	pads             [MaxPads]bool
}

func devSetFor(ids []DeviceID, f *Frame) devSet {
	var ds devSet
	for _, id := range ids {
		switch id {
		case KeyboardDevice:
			ds.kb = true
		case MouseDevice:
			ds.mouse = true
		case TouchDevice:
			ds.touch = true
		default:
			for i := range f.Pads[:f.NPads] {
				if f.Pads[i].Device == id {
					ds.pads[i] = true
				}
			}
		}
	}
	return ds
}

// owner returns the player (or the lobby) that has device id.
func (r *Router) owner(id DeviceID) *Player {
	for _, p := range r.players {
		if slices.Contains(p.devices, id) {
			return p
		}
	}
	if slices.Contains(r.lobby.devices, id) {
		return r.lobby
	}
	return nil
}

func (r *Router) applyDeviceChanges() {
	for _, id := range r.changes.Disconnected {
		i := slices.IndexFunc(r.devices, func(d DeviceInfo) bool { return d.ID == id })
		if i < 0 || id < FirstPadDevice {
			continue
		}
		info := r.devices[i]
		r.devices = slices.Delete(r.devices, i, i+1)
		r.events.DevicesDisconnected = append(r.events.DevicesDisconnected, info)
		p := r.owner(id)
		if p == nil {
			continue
		}
		p.removeDevice(id)
		if p != r.lobby {
			pd := PlayerDevice{Player: p.index, Device: info}
			r.lost = append(r.lost, pd)
			r.events.DevicesLost = append(r.events.DevicesLost, pd)
		}
	}
	for _, info := range r.changes.Connected {
		if info.ID < FirstPadDevice || slices.ContainsFunc(r.devices, func(d DeviceInfo) bool { return d.ID == info.ID }) {
			continue
		}
		info.Class = Gamepad
		if info.Family == FamilyGeneric {
			info.Family = FamilyOf(info.Name)
		}
		r.devices = append(r.devices, info)
		r.events.DevicesConnected = append(r.events.DevicesConnected, info)
		if i := slices.IndexFunc(r.lost, func(pd PlayerDevice) bool {
			return pd.Device.GUID == info.GUID && pd.Device.Name == info.Name
		}); i >= 0 {
			pd := r.lost[i]
			r.lost = slices.Delete(r.lost, i, i+1)
			p := r.players[pd.Player]
			p.devices = append(p.devices, info.ID)
			r.events.DevicesRestored = append(r.events.DevicesRestored, PlayerDevice{Player: pd.Player, Device: info})
			continue
		}
		if r.opts.Assign == AssignSingle {
			r.players[0].devices = append(r.players[0].devices, info.ID)
		} else {
			r.lobby.devices = append(r.lobby.devices, info.ID)
		}
	}
}

// checkJoins assigns unassigned devices that pressed the join action to the
// lowest free player slot. Keyboard and mouse join together.
func (r *Router) checkJoins() {
	for i := 0; i < len(r.lobby.devices); i++ {
		id := r.lobby.devices[i]
		if !r.joinPressed(id) {
			continue
		}
		slot := slices.IndexFunc(r.players, func(p *Player) bool { return !p.joined })
		if slot < 0 {
			return
		}
		p := r.players[slot]
		p.joined = true
		p.Assign(id)
		if id == KeyboardDevice || id == MouseDevice {
			for _, other := range []DeviceID{KeyboardDevice, MouseDevice} {
				if slices.Contains(r.lobby.devices, other) {
					p.Assign(other)
				}
			}
		}
		r.events.PlayersJoined = append(r.events.PlayersJoined, slot)
		i = -1 // the lobby's device list changed; start over
	}
}

// joinPressed reports whether a binding of the join action went down on
// device id alone this tick. Only digital controls count; modifiers are
// ignored.
func (r *Router) joinPressed(id DeviceID) bool {
	ids := [1]DeviceID{id}
	cur, prev := devSetFor(ids[:], &r.frame), devSetFor(ids[:], &r.prev)
	b := r.lobby.eff
	for i := range b.binds {
		bd := &b.binds[i]
		if bd.action != r.opts.JoinAction || bd.unbound || (bd.kind != bindSingle && bd.kind != bindChord) {
			continue
		}
		now, was := true, true
		for _, c := range bd.ctrl[:bd.n] {
			if c.Type == CtrlTouch && c.Code != TouchAny {
				now = false
				break
			}
			x, _ := r.rawValue(c, &cur, &r.frame)
			px, _ := r.rawValue(c, &prev, &r.prev)
			now = now && abs(x) >= bd.threshold
			was = was && abs(px) >= bd.threshold
		}
		if now && !was {
			return true
		}
	}
	return false
}

// Assign gives device id to p, taking it from whoever has it, and marks p
// joined. Unknown devices are ignored.
func (p *Player) Assign(id DeviceID) {
	if _, ok := p.r.Device(id); !ok {
		return
	}
	if o := p.r.owner(id); o != nil {
		o.removeDevice(id)
	}
	p.devices = append(p.devices, id)
	p.joined = true
}

// Unassign returns device id to the lobby.
func (p *Player) Unassign(id DeviceID) {
	if !slices.Contains(p.devices, id) || p == p.r.lobby {
		return
	}
	p.removeDevice(id)
	p.r.lobby.devices = append(p.r.lobby.devices, id)
}

// Leave returns all of p's devices to the lobby, forgets its lost devices
// and frees the slot (PlayersLeft event). The lobby cannot leave.
func (p *Player) Leave() {
	if p == p.r.lobby || !p.joined {
		return
	}
	p.r.lobby.devices = append(p.r.lobby.devices, p.devices...)
	p.devices = p.devices[:0]
	p.r.lost = slices.DeleteFunc(p.r.lost, func(pd PlayerDevice) bool { return pd.Player == p.index })
	p.joined = false
	p.r.events.PlayersLeft = append(p.r.events.PlayersLeft, p.index)
}

// Joined reports whether the slot is taken. With AssignSingle player 0 is
// always joined.
func (p *Player) Joined() bool { return p.joined }

// Devices returns the devices assigned to p. Valid until the next change.
func (p *Player) Devices() []DeviceID { return p.devices }

func (p *Player) removeDevice(id DeviceID) {
	p.devices = slices.DeleteFunc(p.devices, func(d DeviceID) bool { return d == id })
}
