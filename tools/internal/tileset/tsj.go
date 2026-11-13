package tileset

import "encoding/json"

// tsj is the Tiled JSON tileset (the fields tiled reads, plus the
// ones Tiled wants to open it).
type tsj struct {
	Type         string    `json:"type"`
	Version      string    `json:"version"`
	TiledVersion string    `json:"tiledversion"`
	Name         string    `json:"name"`
	Image        string    `json:"image"`
	ImageWidth   int       `json:"imagewidth"`
	ImageHeight  int       `json:"imageheight"`
	TileWidth    int       `json:"tilewidth"`
	TileHeight   int       `json:"tileheight"`
	TileCount    int       `json:"tilecount"`
	Columns      int       `json:"columns"`
	Margin       int       `json:"margin"`
	Spacing      int       `json:"spacing"`
	Tiles        []tsjTile `json:"tiles"`
}

type tsjTile struct {
	ID         int           `json:"id"`
	Properties []tsjProperty `json:"properties"`
}

type tsjProperty struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// EncodeTSJ returns the Tiled JSON tileset for the laid-out atlas. image is
// the PNG's path relative to the tileset file. Every tile gets a string
// property "name" with its sub-texture name.
func EncodeTSJ(a *Atlas, l Layout, name, image string) ([]byte, error) {
	size := l.Size()
	t := tsj{
		Type: "tileset", Version: "1.10", TiledVersion: "1.10.2",
		Name: name, Image: image, ImageWidth: size.X, ImageHeight: size.Y,
		TileWidth: l.Tile.X, TileHeight: l.Tile.Y,
		TileCount: l.Count, Columns: l.Columns, Margin: l.Margin, Spacing: l.Spacing,
	}
	for i, s := range a.SubTextures {
		t.Tiles = append(t.Tiles, tsjTile{ID: i, Properties: []tsjProperty{{"name", "string", s.Name}}})
	}
	b, err := json.MarshalIndent(t, "", "  ")
	return append(b, '\n'), err
}
