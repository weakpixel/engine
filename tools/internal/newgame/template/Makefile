# The game's everyday commands. Every tool runs through "go tool weakpixel",
# at the engine version in go.mod.

GOROOT := $(shell go env GOROOT)

.PHONY: run dev generate test check doc web web-packed web-dev editor serve clean

# The game in a window, with the embedded assets.
run:
	go run .

# The game reading assets/ from disk: saved changes reload while it runs.
dev:
	go run . -assets assets

# Game Object code (game/zz__gen.go) and GOD files (build/god/, not committed) from the +game markers.
generate:
	go generate ./...

# The game's tests, and the editor build's (editor_test.go).
test:
	go test ./...
	go test -tags editor .

# Checks the asset library: file names, groups, broken references.
check:
	go tool weakpixel assets check

# The merged GOD (build/god/project.god.json) and HTML docs of every Game Object (build/god-doc).
doc:
	go tool weakpixel doc --format god
	go tool weakpixel doc

# The browser build in build/web, assets embedded. Serve it with make serve.
web:
	mkdir -p build/web
	GOOS=js GOARCH=wasm go build -o build/web/game.wasm .
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" web/index.html build/web/

# The browser build in build/web-packed: the assets as packs, fetched over HTTP.
web-packed:
	rm -rf build/web-packed
	mkdir -p build/web-packed
	GOOS=js GOARCH=wasm go build -tags webassets -o build/web-packed/game.wasm .
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" web/index.html build/web-packed/
	go run . -pack build/web-packed/assets

# The browser build for development, at http://localhost:8080: it reads the loose
# files of assets/ from a dev server that pushes changes, so a file you save
# (a shader, a texture, a map) reloads in the open page without a refresh.
web-dev:
	rm -rf build/web-dev
	mkdir -p build/web-dev
	GOOS=js GOARCH=wasm go build -tags webassets,webdev -o build/web-dev/game.wasm .
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" web/index.html build/web-dev/
	go tool weakpixel serve --assets assets build/web-dev

# The level editor in a window, without VSCode.
editor:
	go run -tags editor .

# Serves build/web on http://localhost:8080 (any static file server works).
serve:
	python3 -m http.server 8080 -d build/web

clean:
	rm -rf build
