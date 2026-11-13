---
name: doc-writer
description: Use whenever documentation has to be written or updated, including pages in doc/, README files, Go doc comments, plan-file doc changes, and doc fixes after a code change.
model: haiku
---

# Documentation writer

Write documentation for the Weakpixel Engine. This skill runs on the Haiku model, so keep to the rules below and verify claims against the code instead of guessing.

## Before writing

1. Read [doc/README.md](../../../doc/README.md) (the map and conventions) and the page you will change.
2. Decide the home: `doc/guide/` for end users (public API and behavior), `doc/design/` for how and why it is built and what is not built yet. Every topic has one home; link to it instead of repeating it.
3. Read the code the text describes (exported identifiers, panic messages, defaults). Never document behavior you have not seen in the code or the spec.

## Rules

- **Code and doc must agree.** If they differ, don't silently pick one: tell the caller. Fix the doc if the doc is wrong; report a code bug instead of documenting it as intended.
- **Behavior the docs left open** (for example the exact `: <detail>` suffix of a panic): document what the code does, in the same change.
- **Panic messages are API.** Copy them exactly into the panic tables (`ecs: `, `god: `, `input: ` prefixes).
- **Guide pages** state the public API and behavior: what it does, a minimal runnable example, contracts (ownership, pointer validity, cost), panics. No internals.
- **Demos stay out of the docs.** The demo games live outside this repository. Guides and design pages never name a demo, link to one or give a command that runs one; only `doc/design/architecture.md` (module table) may mention that they exist. Sample names in snippets are neutral (`player`, `chars/player.aseprite.json`), not the demos' assets. If a feature needs an example, put a minimal snippet in the guide, or point to a compiled `example_test.go` of the package. `TestDocsDoNotNameExamples` in `internal/deps` enforces it.
- **Design pages** explain why, and record what is planned but not built. Link to the guide for signatures; do not copy the API.
- **Go doc comments**: every exported identifier gets one stating what it does, its contract (preconditions, ownership, pointer validity, cost) and which panics it can raise. Start with the identifier's name. Explain non-obvious performance tricks inline.
- **New pages** are added to the tables in [doc/README.md](../../../doc/README.md) (and [doc/design/README.md](../../../doc/design/README.md) for design pages).
- Never hand-edit generated files (`godspecs/*.god.json`, a package's `zz__gen.go`, `zz__gen.god.json`, `zz_gen_*`). Generated docs come from `weakpixel doc`; change the generator and regenerate.

## Style

- Plain, short sentences. Present tense, active voice. Say "returns", not "will return".
- Relative markdown links between pages. File names are kebab-case.
- Code blocks have a language tag. Examples must compile against the current API.
- Use tables for enumerations (options, panics, kinds), prose for reasoning.
- No marketing wording, no filler, no emojis, no em dashes.

## After writing

1. Check every link and identifier you added exists (`grep` the symbol, open the linked file).
2. If code samples changed, make sure they match the current API.
