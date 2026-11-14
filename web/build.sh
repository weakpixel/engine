#!/bin/sh
# Builds the main package in <pkgdir> for the browser into <outdir>:
# game.wasm, Go's wasm_exec.js and the index.html shell. Works from any
# directory, for a package of any module:
#
#   web/build.sh ./mygame build/web/mygame
#
# With -packed the game is built with the webassets tag, so it embeds no
# assets, and "go run . -pack <outdir>/assets" in <pkgdir> writes its packed
# asset library next to the page (doc/guide/engine/web.md). Only games that
# support it take -packed.
#
# Serve <outdir> with any static file server (wasm needs http://, not file://).
set -eu
tags=""
packed=false
if [ "${1:-}" = "-packed" ]; then
	packed=true
	tags="webassets"
	shift
fi
if [ $# -ne 2 ]; then
	echo "usage: build.sh [-packed] <pkgdir> <outdir>" >&2
	exit 2
fi
pkg=$1
out=$2
here=$(dirname "$0")
mkdir -p "$out"
out=$(cd "$out" && pwd)
GOOS=js GOARCH=wasm go -C "$pkg" build -tags "$tags" -o "$out/game.wasm" .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$out/"
cp "$here/index.html" "$out/"
if $packed; then
	rm -rf "$out/assets"
	go -C "$pkg" run . -pack "$out/assets"
fi
