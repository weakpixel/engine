---
name: engine-status
description: Use to check what is built, partial or not started in an area of the Weakpixel engine, or what to do next; cross-checks the design README, plan files and code.
---

# Weakpixel engine status

The docs in `doc/` are the spec (see CLAUDE.md). Use this skill to identify current work, not as a second status ledger.

## Check status first

Before advising what to implement next:

- Read `doc/design/README.md` for the built/not-built summary, then only the one design page for the area and the guide section it links.
- Read the Status line of each plan (`grep -m1 Status` over the plan folder) and open only the related plans; resume existing work rather than proposing it again.
- Check the relevant code and tests when a status is unclear or a plan's verification needs confirming. Don't infer current state from this skill or an old checklist.

For a status report, summarize the relevant area as built, partial, or not started, citing the design page and any active plan.

## Working rules

- The ECS core (`World`, views, queries, resources, `system`, the arity generator) lives in the `weakpixel/ecs` repository, module `github.com/weakpixel/ecs`. Core work is planned and merged there first; the engine then bumps the version in every `go.mod` that requires it. This repository keeps `god`, `wpix gen`, the tools and the Engine Runtime; the Lua bindings live in `weakpixel/contrib` (`lua/`).
- Carry out non-trivial implementation through `/plan` → `/implement`; keep the plan's scope and acceptance criteria authoritative.
- Work in the order supported by current docs and plans. Use the relevant domain skill for implementation details and tests.
- When documentation leaves a behavior decision open, record it in the plan and the owning design/guide page.
- Report current status, remaining work, and decisions needed; don't recommend a fixed next phase from a historical checklist.
