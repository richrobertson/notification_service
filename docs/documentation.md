# Documentation Guide

## Purpose

This repository uses Go doc comments as the source of truth for API-facing code documentation.

The goal is to make the docs useful in two passes:

- the first sentence should be a terse reference summary for experienced readers
- the rest of the comment should add enough context, invariants, and examples to help new contributors understand why the code exists and how it is meant to be used

## Writing style

When documenting exported code:

1. Start with the symbol name and a short summary sentence.
2. Add one or two short paragraphs that explain:
   - the symbol's role in the system
   - important invariants or side effects
   - when a caller should or should not use it
3. Add small examples only when they clarify a workflow that is otherwise easy to misuse.
4. Prefer practical operational wording over abstract theory.
5. Be explicit about limits. If something is best-effort, at-least-once, compatibility-only, or operator-facing, say so.

## What must be documented

Maintain this standard for:

- every package
- every exported type
- every exported function
- every exported method
- exported constants and variables when they are part of the package contract

If a new exported symbol is added, its doc comment should be added in the same change.

## Keep docs in sync

Before merging code that changes runtime behavior:

1. update the Go doc comments on the affected package and exported symbols
2. update user-facing docs such as `README.md` or `docs/runbook.md` when operator behavior changes
3. regenerate the API docs section:

```bash
make docs-godoc
```

The generated files land in `docs/api/` and are meant to be committed when the exported API or package docs change in a meaningful way.

## Review guidance for maintainers

When reviewing a PR, ask:

- Would a new contributor understand how to use this symbol safely?
- Would an experienced maintainer be able to skim the first sentence as a quick reference?
- Does the comment describe real behavior, not idealized behavior?
- If the code has operational caveats, are they documented?

If the answer is no, request a doc update in the same PR rather than treating documentation as follow-up work.
