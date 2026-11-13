package goddoc

import "github.com/weakpixel/engine/god"

// Load reads the GOD files named by inputs (files, directories and "..."
// patterns, as for `weakpixel doc`) and returns the merged components and
// objects, sorted by key. Objects carry no component schemas; look them up in
// the components by key. All problems are reported together.
func Load(inputs []string) ([]god.ComponentSchema, []god.Schema, error) {
	files, err := collect(inputs)
	if err != nil {
		return nil, nil, err
	}
	doc, err := loadDocument("", files)
	if err != nil {
		return nil, nil, err
	}
	merged := doc.merged()
	return merged.Components, merged.Objects, nil
}
