module extmod

go 1.27

require (
	github.com/weakpixel/ecs v0.0.1
	github.com/weakpixel/engine v0.0.0-00010101000000-000000000000
)

tool github.com/weakpixel/engine/tools/wpix

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/weakpixel/asset v0.0.1 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

replace github.com/weakpixel/engine => ../../../../..
