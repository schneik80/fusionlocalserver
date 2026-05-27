# Where-Used on v3 — investigation & current state

**Status: deferred (2026-05-27).** Where-Used currently returns an empty list on
v3. Left as-is pending offline research into whether v3 exposes a usable
reverse-reference mechanism. This note records where we left it.

## What the feature is

"Where Used" answers: *which parent designs/assemblies consume this component?*
In the v2 Manufacturing Data Model this was a first-class field —
`componentVersion.whereUsed` returned the parent component versions, which we
de-duplicated to one row per owning `DesignItem`.

## What changed in v3

`ComponentVersion` no longer exists, and **there is no reverse-reference query
in v3**. Confirmed on 2026-05-27 by live `__type` introspection of the v3
endpoint (`/mfg/v3/graphql/public`) via a temporary `/api/_introspect-v3` route
(since removed — see Pointers below):

- Root `Query` has **no** `whereUsed` / `usedIn` / `referencedBy` / reverse
  query. Relationship traversal is forward-only:
  - `Component.bomRelations(depth, pagination)` — direct children (used by Uses
    + BOM).
  - `Model.assemblyRelations(depth, pagination, scope)` — the model's **downward**
    assembly sub-tree. `AssemblyRelation { fromModel, toModel, … }`; `scope` is
    `AssemblyRelationScopeInput` enum `{ FULL, INTERNAL_PLUS_DIRECT_EXTERNAL }`
    (controls tree breadth, **not** direction).
- `Component`, `Model`, and `DesignItem` have **no** where-used / parent field
  (full field lists captured in the introspection run; see
  [[v3-api-schema-facts]] in agent memory).
- The official reference app `github.com/tapnair/fusion-data-demo-v3` **does not
  implement where-used** either.

## Why the current implementation returns empty

`api/refs.go` `GetWhereUsed` attempts the only schema-plausible path: read the
component's `primaryModel.assemblyRelations` and keep relations where this model
is the `toModel` (so each `fromModel` is a parent). Live, `assemblyRelations`
returns `results: []` for every component tested — because it is a *downward*
traversal (a leaf part has no children; an assembly lists its own children as
`toModel`, never itself). So the `toModel == self` filter never matches, and the
tab shows no results. No GraphQL error — just an empty result.

The code and the Where-Used tab are intentionally **left in place** (returning
empty) rather than removed, so the feature can be wired up quickly if research
turns up a viable query.

## Options when we resume

- **(a) Hide the Where-Used tab on v3** — cleanest if v3 has no reverse query.
- **(b) A specific v3 query** — the working hypothesis (per an external expert)
  is that assembly/BOM relations or a time-resolved component query
  ("getComponentAtTime") can yield parents. Introspection did not reveal such a
  field; **the thing to find offline is the exact query/field**. If it exists,
  wiring it into `GetWhereUsed` is small.
- **(c) Hub-wide scan** — enumerate every design in the hub, walk each one's
  full `assemblyRelations` tree (`scope: FULL`), and invert to find which
  reference the target. Correct but expensive (heavier than the drawings scan in
  `GetDrawingsForDesign`); only viable with caching/precomputation.

## Pointers

- Implementation: `api/refs.go` → `GetWhereUsed` (and `v3FromModelFields`).
- Schema oracle: the temporary `GET /api/_introspect-v3?type=<TypeName>` route
  (`server/handlers_introspect.go`) used during the investigation has since been
  removed; rebuild it from git history if another introspection pass is needed.
- Full v3 schema findings: agent memory `v3-api-schema-facts.md`.
