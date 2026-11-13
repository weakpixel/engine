// Package ext is a foreign component package. A second package also named
// "ext" (testdata/golden/full/ext2dir) forces the generator's import-alias
// clash handling ("ext", "ext2").
package ext

// Alpha is a foreign component with its identity on the type.
//
// +game:component:group=extgrp
type Alpha struct{ V int }
