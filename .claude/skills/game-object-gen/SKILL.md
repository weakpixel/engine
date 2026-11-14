---
name: game-object-gen
description: Use for weakpixel gen, +game markers, generated Game Object views, GOD schemas/serialization, or generator tests.
---

# Game Object generator and GOD runtime

The specs are [the Game Object API](../../../doc/guide/ecs/api-game-object.md) and [the GOD definition](../../../doc/guide/ecs/game-object-definition.md). Use those pages for complete marker, runtime, schema, and serialization requirements; this skill focuses on generator internals and the workflow for changing them.

## Generator pipeline

```
main.go             run -> generate (no filesystem writes) -> write outputs
load.go             load target/dependency packages; hide old generated Go through an overlay
markers.go, parse.go define markers and flatten embedded objects
validate.go         collect and sort source-positioned diagnostics
schema.go, render.go build GOD schemas and typed template data
generator.go.tpl    emit zz__gen.go; render GOD JSON to <specs>/<group>.god.json
                    (specs folder: god.specs, default build/god)
weakpixel doc       merge GOD files; Markdown, HTML and --format god output
```

`generate(dir, patterns)` is the testable, side-effect-free boundary. Preserve declaration order in GOD component lists and deterministic sorted field order in template data. Objects with 1..16 required components use `ecs.ViewN`; zero or more than 16 use `World.Query(ids...)` and `[]any`. Optional fields use low-level `World` methods. Generated IDs include foreign package aliases (for example, `CID_ExtHitbox`).

## Contract with ecs

The template may use only exported API of the `github.com/weakpixel/ecs` module ([guide](https://github.com/weakpixel/ecs/blob/main/doc/README.md)), so generated code compiles from other modules (`extmod`). `ViewN` values share state when copied; in-place writes and no-op removals must not bump the revision. Untyped iteration uses `World.Query(ids...)`; `World.Query()` with no IDs matches every entity. There is no `Selector` type.

Keep the generated view accessor (`<Object>View` / `New<Object>View`) distinct from `god.Schema`, which describes a JSON object schema. The API guide owns the generated method and behavior contract. When a template change affects output, regenerate it and test the generator and consumers; if an ECS API used by the template changes, that change merges in `weakpixel/ecs` first and the engine PR that bumps the module version updates the template.

## Changing a marker or rule

1. Define the marker in `markers.go`; parse and flatten it in `parse.go`.
2. Validate in `validate.go`, collect all issues through `p.errorf`, and keep diagnostics source-positioned and deterministic.
3. Update `render.go` or the template when output changes; bump GOD `version` only for incompatible format changes.
4. Add or extend golden and diagnostic tests, and update the owning guide page for user-visible marker behavior.

Documentation extracted from Go comments belongs in the package's GOD file (`<specs>/<group>.god.json`); generated Go schemas omit descriptions. `weakpixel doc` consumes that JSON. Keep schema/runtime behavior aligned with the GOD definition guide.

## Tests

`weakpixel gen` has golden, diagnostic, stale-file, documentation, and external-module (`extmod`) tests. Extend the narrowest relevant test; preserve coverage for aggregated, deterministic diagnostics, output stability, stale-file ownership, and generation from an external module. Run `go test ./tools/internal/gen/...`; for generated consumer changes, also run `go generate ./...` and build/test affected packages.

Before describing a runtime behavior as missing, check the guide and the current `testdata/gametypes` and `god` implementation. The docs own the complete runtime behavior checklist; don't keep completed migration tasks here.
