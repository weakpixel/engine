package tween

// ID is the handle of one tween. It stays valid while the tween runs
// (including its delay, and while paused or waiting in a sequence); once the
// tween finishes or is killed, every call with the ID is a no-op, so
// gameplay code never has to track whether a tween still runs. The zero ID
// is never valid.
type ID struct {
	index uint32
	gen   uint32
}

// IsZero reports whether id is the zero ID.
func (id ID) IsZero() bool { return id == ID{} }

// slot locates a running tween: the pool of its lens and its row there.
// Swap-remove moves rows, so the pool updates row when it moves one.
type slot struct {
	gen  uint32
	lens uint32 // pool index in Tweens.pools; 0 when the slot is free
	row  int32
}

// slots is the generational table behind ID. A freed slot bumps its
// generation, which makes every ID that pointed to it stale.
type slots struct {
	s    []slot
	free []uint32
}

// alloc returns a new ID for a tween in lens's pool at row.
func (t *slots) alloc(lens uint32, row int32) ID {
	var idx uint32
	if n := len(t.free); n > 0 {
		idx, t.free = t.free[n-1], t.free[:n-1]
	} else {
		idx = uint32(len(t.s))
		// Generation 0 is never used, so the zero ID is never valid.
		t.s = append(t.s, slot{gen: 1})
	}
	sl := &t.s[idx]
	sl.lens, sl.row = lens, row
	return ID{index: idx, gen: sl.gen}
}

// get returns the slot of a live id, or nil.
func (t *slots) get(id ID) *slot {
	if int(id.index) >= len(t.s) {
		return nil
	}
	sl := &t.s[id.index]
	if sl.gen != id.gen || sl.lens == 0 {
		return nil
	}
	return sl
}

// release frees id's slot. id must be live.
func (t *slots) release(id ID) {
	sl := &t.s[id.index]
	sl.lens = 0
	sl.gen++
	if sl.gen == 0 {
		sl.gen = 1
	}
	t.free = append(t.free, id.index)
}

// clear frees every live slot.
func (t *slots) clear() {
	for i := range t.s {
		if t.s[i].lens != 0 {
			t.release(ID{index: uint32(i), gen: t.s[i].gen})
		}
	}
}

// live returns the number of live IDs.
func (t *slots) live() int { return len(t.s) - len(t.free) }
