package speech

import "github.com/weakpixel/voice"

// owners hands out the voice.Owner of every speaking entity. The synth keeps
// the patches of an owner until they are replaced, so the pool remembers
// which voice each owner was patched with: a reused owner is patched again
// only when its new voice differs.
type owners struct {
	free    []voice.Owner
	next    voice.Owner
	patched []*voice.Voice // by owner: the voice the sink has for it, or nil
}

// take returns a free owner, preferring one returned earlier.
//
// Panics with "speech: more than 65536 speakers" when every owner is taken.
func (o *owners) take() voice.Owner {
	if n := len(o.free); n > 0 {
		id := o.free[n-1]
		o.free = o.free[:n-1]
		return id
	}
	if int(o.next) < len(o.patched) {
		panic("speech: more than 65536 speakers")
	}
	id := o.next
	o.next++
	o.patched = append(o.patched, nil)
	return id
}

// give returns id to the pool. Its patches stay in the sink and are reused
// by the next taker with the same voice.
func (o *owners) give(id voice.Owner) { o.free = append(o.free, id) }

// needsPatch reports whether id must be patched for v, and records that it
// will be.
func (o *owners) needsPatch(id voice.Owner, v *voice.Voice) bool {
	if o.patched[id] == v {
		return false
	}
	o.patched[id] = v
	return true
}

// forget marks id as unpatched (its SetVoice failed).
func (o *owners) forget(id voice.Owner) { o.patched[id] = nil }

// inUse is the number of owners taken and not given back.
func (o *owners) inUse() int { return int(o.next) - len(o.free) }
