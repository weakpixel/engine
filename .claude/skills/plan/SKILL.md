---
name: plan
description: Use when the user requests a scoped implementation plan in plan/; creates or updates one plan without code changes.
argument-hint: <what to plan>
disable-model-invocation: true
---

# /plan: write an implementation plan

Request: $ARGUMENTS

Produce **one plan file** at `plan/<plan-slug>.plan.md` (relative to the workspace root) that `/implement` can carry out without re-deriving context. **Don't change any code, tests, docs or generated files in this step.** The plan file is the only output.

## Where the plan goes

If the project instructions or project memory name a plan folder (for example a shared folder outside the repo), use that folder instead of `plan/` everywhere below; it is pre-approved in the same way.

Write the plan **directly** to `<project>/plan/<plan-slug>.plan.md` with the Write tool (create `plan/` if it's missing). This is pre-approved: don't ask for permission first. Never write the plan to `$HOME/.claude/plans/` or any other location outside the project, and don't use EnterPlanMode/ExitPlanMode for it. Those tools route the plan file into `$HOME/.claude/`. This skill is the planning workflow. Writes under `plan/` are the only file changes allowed in this step.

## Steps

1. **Understand the request.** If `$ARGUMENTS` is empty, ask what to plan. If the request is about what to build next, load the `engine-status` skill first.
2. **Load context.** Read the relevant `doc/*.md` pages (they are the spec) and load the domain skills that apply (for example `game-object-gen`). Read the code the plan will touch, and check what already exists rather than trusting the status column of doc/design/README.md.
3. **Check existing plans.** `ls plan/`. If a plan already covers this request, update that file instead of creating a duplicate, and tell the user.
4. **Resolve decisions.** For anything the docs leave open, recommend an option. Ask the user (AskUserQuestion) only when the choice really is theirs and changes the plan. Record every decision in the plan, including the ones you made yourself.
5. **Write the plan** using the template below. Choose a short kebab-case slug that describes the outcome (`asset-packages`, `typed-query-allocs`), not the date.
6. **Report:** the file path, a 3–5 line summary, and any open questions. Tell the user to run `/implement <plan-slug>` when they're ready.

## Plan rules

- Steps are small, ordered and independently verifiable. Each step names the files it touches and how to check it (a test name, a command). Put tests in the same step as the code they cover.
- Make the scope explicit: list what's out of scope so `/implement` doesn't drift.
- Write acceptance criteria that can be checked: commands that must pass, behaviors with a named test, doc sections that must be updated.
- Reference the spec by file and section (`doc/guide/engine/assets.md#hot-reload`) instead of copying it.
- If the work is too big for one session (roughly 15 steps or more), split it into several plans and link them under **Related plans**.
- Every plan's last step is: run `make all`, then update the docs for any decisions or API changes.

## Template

```markdown
# <Title>

Status: draft | ready | in-progress | blocked | done
Created: <YYYY-MM-DD>
Roadmap phase: <phase number and name, or "n/a">
Related plans: <slugs or "none">

## Goal
<1–3 sentences: what will be true when this plan is done.>

## Context
- Spec: <doc links with anchors>
- Skills: <skills /implement should load>
- Existing code: <files/functions it builds on>

## Decisions
- <decision> — <why>. (Documented in: <doc file> | to document in step N)

## Out of scope
- <item>

## Steps
- [ ] 1. <action> — files: <paths> — verify: <command or test name>
- [ ] 2. ...
- [ ] N. Run `make all`; update docs for decisions/API changes — verify: clean `make all`, no generate diff

## Acceptance criteria
- [ ] <checkable criterion>

## Open questions
- <question, or "none">

## Implementation log
- <date>: /implement appends dated entries here: progress, deviations, follow-ups
```

Set `Status: ready` when there are no open questions, otherwise `draft`.
