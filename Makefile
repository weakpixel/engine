.PHONY: all
all:
	go generate ./...
	go fmt ./...
	go build ./...
	go test ./...
	go vet ./...
	$(MAKE) test-ebiten

# Pixel-reading tests that need a display (xvfb-run on a headless machine).
.PHONY: test-ebiten
test-ebiten:
	go test -tags ebitentest ./...

.PHONY: modtidy
modtidy:
	go mod tidy

.PHONY: race
race:
	go test -race ./...

.PHONY: vet
vet:
	go vet ./...

# Benchmarks of god: `make bench name=baseline`, then after a change
# `make bench name=phase1 && make benchcmp new=phase1`.
name ?= current
new ?= current
count ?= 10

.PHONY: bench
bench:
	mkdir -p bench
	go test ./god/... -run '^$$' -bench . -benchmem -count $(count) | tee bench/$(name).txt

.PHONY: benchcmp
benchcmp:
	@test -f bench/baseline.txt || { echo "benchcmp: no bench/baseline.txt, run make bench name=baseline first"; exit 1; }
	benchstat bench/baseline.txt bench/$(new).txt

# Benchmarks of the engine packages (bench/engine-baseline.txt is the reference).
.PHONY: engine-bench
engine-bench:
	mkdir -p bench
	go test ./... -run '^$$' -bench . -benchmem -count $(count) | tee bench/engine-$(name).txt

# The merged project GOD (god.out in .weakpixel.json, under the ignored
# build/) and the documentation of all Game Objects and Components in
# god.specs, as one HTML page and one Markdown file in godDoc.out.
.PHONY: god-doc
god-doc:
	go run ./tools/weakpixel doc --format god
	go run ./tools/weakpixel doc
	go run ./tools/weakpixel doc --format md

.PHONY: build-tools
build-tools:
	go build -o ./build/weakpixel ./tools/weakpixel

# js/wasm. WASM_ENV runs go test under go_js_wasm_exec (Node): it puts the
# exec wrapper on PATH and starts from an empty environment, because Node
# refuses a command line plus environment over its size limit, which a
# large CI or container environment exceeds.
WASM_ENV = env -i HOME="$(HOME)" PATH="$(shell go env GOROOT)/lib/wasm:$(PATH)" \
	GOPATH="$(shell go env GOPATH)" GOMODCACHE="$(shell go env GOMODCACHE)" \
	GOCACHE="$(shell go env GOCACHE)" GOTOOLCHAIN="$(shell go env GOTOOLCHAIN)" \
	GOFLAGS=-mod=mod GOOS=js GOARCH=wasm

.PHONY: wasm
wasm: wasm-build wasm-test

# Cross-compiles the module for the browser (doc/guide/engine/web.md), and the
# editor test game with the editor tag.
.PHONY: wasm-build
wasm-build:
	GOOS=js GOARCH=wasm go build ./...
	GOOS=js GOARCH=wasm go build -tags editor -o build/editor-testgame.wasm ./tools/weakpixel-editor/testdata/game

# Runs the tests that work without a browser under js/wasm (needs Node; slow,
# so not part of all).
.PHONY: wasm-test
wasm-test:
	@command -v node >/dev/null || { echo "wasm-test: node not found, skipped"; exit 0; }
	$(WASM_ENV) go test ./god/...
	$(WASM_ENV) go test ./asset/library/... ./anim/animfile ./tiled ./input ./gfx ./sound ./tween
