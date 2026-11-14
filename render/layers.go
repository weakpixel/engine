package render

// layerBits is the number of int16 layers; one bit per layer.
const layerBits = 1 << 16

// LayerOptions configures how one layer is sorted.
type LayerOptions struct {
	// YSort orders items of the layer by Item.SortY (top-down games: things
	// lower on the screen draw in front).
	YSort bool
}

// LayerConfig holds per-layer options. The zero value is ready to use and
// means "no layer is Y-sorted". It is read while sorting, so configure it
// before the first tick rather than during one.
type LayerConfig struct {
	// ySort is a bitset indexed by layer+32768: a lookup is one load, which
	// matters because the sort comparator asks for it on every comparison.
	ySort [layerBits / 64]uint64
}

// Set applies opts to layer.
func (c *LayerConfig) Set(layer int16, opts LayerOptions) {
	word, bit := layerIndex(layer)
	if opts.YSort {
		c.ySort[word] |= bit
	} else {
		c.ySort[word] &^= bit
	}
}

// Options returns the options of layer.
func (c *LayerConfig) Options(layer int16) LayerOptions {
	return LayerOptions{YSort: c.ySortLayer(layer)}
}

func (c *LayerConfig) ySortLayer(layer int16) bool {
	if c == nil {
		return false
	}
	word, bit := layerIndex(layer)
	return c.ySort[word]&bit != 0
}

func layerIndex(layer int16) (word int, bit uint64) {
	i := int(layer) + 1<<15
	return i / 64, 1 << (i % 64)
}
