# Value types for structured non-entity values

Add a top-level `types:` section for named value types: structured values with
internal shape and validity rules that are not entities. Resolves issue #11.

## Context

Attribute types were primitives or enum names; relationship targets were
entities. A *value type* — a structured value with internal shape and rules but
no identity and no lifecycle — had no home. A `Money` value (an amount and a
currency, with its own validity rules) could only live in the glossary as
prose; promoting it to an entity would give it identity and ER presence it does
not deserve. Lists had no home at all. This is also the "value objects" gap the
schema reference documents as a deliberate omission (issue #4, sub-item 1).

## Decision

A top-level `types:` section, sibling to `enums`. Each named value type has
`fields` (each a name plus a type — a primitive, an enum, or another value
type) and an optional prose `constraints` string. An attribute `type` resolves
against value types too. Collections use `repeated: true` on the attribute.

## The line held

Value types carry fields and *prose* constraints. No expressions, no methods,
no validation grammar. Structure is declarable; rules stay prose. This is the
same communication-first stance that keeps modelith out of IDL territory (see
also ADR-0006). Value-type fields may themselves be value types; no recursion.

## Considered options

- **`repeated: true` vs a `list of X` type string** — chose `repeated: true` so
  type resolution stays a single name lookup rather than parsing a type
  expression.
- **`types:` vs `values:` as the section name** — chose `types:` as the plainer
  word; revisit if it reads as broader than intended.

## Consequences

- Value types are not entities, so they do not appear in the ER (per ADR-0002);
  they get their own Markdown section, like enums.
- The existing lint warning for a PascalCase attribute type that names no enum
  extends to resolve value types too.
- This is the largest surface in the pass (new top-level section, `$def`,
  struct, attribute resolution, render, lint) and ships last, on its own.

The enforcing tests (`TestADR_0005_*`) and full implementation land with #11.
