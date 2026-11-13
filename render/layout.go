package render

// Layout computes the logical screen size to return from Ebiten's Layout
// method.
type Layout struct {
	// Width and Height are the logical resolution in pixels.
	Width, Height int
	// Integer makes the image scale by a whole number (pixel-perfect
	// pixel art): the logical size grows to fill the window at the largest
	// integer scale that still fits Width x Height.
	Integer bool
}

// Size returns the logical size for a window of outsideW x outsideH. Pass the
// result to the Screen resource so extract systems cull against it.
func (l Layout) Size(outsideW, outsideH int) (w, h int) {
	if !l.Integer {
		return l.Width, l.Height
	}
	scale := max(1, min(outsideW/l.Width, outsideH/l.Height))
	return outsideW / scale, outsideH / scale
}
