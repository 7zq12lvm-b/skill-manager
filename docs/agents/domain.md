# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- `CONTEXT.md` at the repo root, or
- `CONTEXT-MAP.md` at the repo root if it exists and points at one `CONTEXT.md` per context
- `docs/adr/` for decisions that touch the area being changed

If these files do not exist, proceed silently. The domain docs can be created later when terminology or architectural decisions become settled.

## File structure

This repo currently uses a single-context layout.

## Use the glossary's vocabulary

When output names a domain concept, use the term as defined in `CONTEXT.md`. If the concept does not exist yet, use the clearest product term from the current PRD.

## Flag ADR conflicts

If future work contradicts an existing ADR, surface that conflict explicitly rather than silently overriding it.
