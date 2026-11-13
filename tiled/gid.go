package tiled

// Flip holds the transformation flags Tiled stores in the high bits of a
// global tile ID (GID).
type Flip uint8

// Flip flags. A tile is transformed by applying FlipD (transpose: swap x and
// y), then FlipH, then FlipV. FlipHex is the 120° rotation of hexagonal maps;
// orthogonal maps never set it, and the engine ignores it.
const (
	FlipH Flip = 1 << iota
	FlipV
	FlipD
	FlipHex
)

// Raw GID bits, as stored in tile layer data and tile objects.
const (
	gidFlipH   uint32 = 0x80000000
	gidFlipV   uint32 = 0x40000000
	gidFlipD   uint32 = 0x20000000
	gidFlipHex uint32 = 0x10000000

	// GIDMask clears the flag bits of a raw GID.
	GIDMask uint32 = 0x0fffffff
)

// SplitGID separates a raw GID into the tile's GID (0 means "no tile") and
// its flip flags.
func SplitGID(raw uint32) (gid uint32, f Flip) {
	if raw&gidFlipH != 0 {
		f |= FlipH
	}
	if raw&gidFlipV != 0 {
		f |= FlipV
	}
	if raw&gidFlipD != 0 {
		f |= FlipD
	}
	if raw&gidFlipHex != 0 {
		f |= FlipHex
	}
	return raw & GIDMask, f
}

// JoinGID is the inverse of [SplitGID].
func JoinGID(gid uint32, f Flip) uint32 {
	raw := gid & GIDMask
	if f&FlipH != 0 {
		raw |= gidFlipH
	}
	if f&FlipV != 0 {
		raw |= gidFlipV
	}
	if f&FlipD != 0 {
		raw |= gidFlipD
	}
	if f&FlipHex != 0 {
		raw |= gidFlipHex
	}
	return raw
}
