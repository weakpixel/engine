// +game:group:name=ext2
//
// Package ext (directory ext2dir to keep the import path distinct) shares
// its package name with testdata/golden/full/ext, forcing an alias clash.
package ext

// Beta is a foreign component from the second "ext"-named package.
type Beta struct{ V int }
