# Changelog

All notable changes to `go-envelopes` are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/) and the package
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- **`TypeSupport`, `Registry.CheckSupport` and `Registry.SupportedTypes` — a
  consumer declares which envelope types it handles, BY NAME.** Every
  registered type must be claimed: supported, or excluded with a reason. A type
  that is neither comes back as `*SupportGap.Unclaimed`, and `CheckSupport`
  returns an error.

  Excluding a type by leaving it out of a list is indistinguishable from never
  having heard of it, so a deliberately-rejected type returns silently the next
  time a consumer regenerates against a newer catalog. A named exclusion
  carrying a reason survives regeneration and makes adopting a new type a
  decision someone writes down. Run from a consumer test, a library upgrade that
  adds a type becomes a failing build rather than a silent import.

  The motivating case: `subagent-spawn-approval` and `chat-loop-terminated` are
  boundary violations for a host that is neither an agent launcher nor a session
  manager, and must stay excludable across catalog regenerations.

  **No catalog partition ships, and that is a finding rather than an omission.**
  The wire-kind versus pure-composition split this mechanism was expected to
  expose does not exist in this catalog:

  - No core schema references another type's schema. Every `$ref` is a local
    `#/$defs` pointer, so no type is structurally nested inside another.
  - Types believed to be composition-only are not. `list-card` and
    `confirmation-card` are emitted standalone as whole interactions — that is
    what the retirements of `todo-list` and `plan-review` did, and what their
    `data_source` pointers exist to support.

  All core types are wire kinds. The distinction that *does* exist is **emission
  authority** — whether an agent, a host decision flow, or the runtime may emit
  a given type — which classifies the current 18 as 10 agent-emittable, 3 host
  decision-flow, 4 runtime-emitted and 1 backend-only. That axis is host policy,
  not wire: the host that owns the emission path owns the answer, and one host's
  allow-list is not another's. Encoding it here would repeat the mistake of the
  `component` field removed in this same release — a shared contract asserting a
  single host's local arrangement. So the library supplies the mechanism for a
  consumer to state and verify its own policy, and asserts none of its own.

- **Typed return channels on `Response`: `Answers []Answer` and
  `Decisions []Decision`.** An interactive envelope returns up to three
  different things — freeform data, answers to questions, dispositions of items
  — and collapsing them into one untyped `Payload` loses the distinction where a
  consumer needs it. Both are optional and additive; a response that carries
  only `Payload` behaves exactly as before.

  `Answer.AcceptedSuggestion` is a `*bool` so that an explicit rejection is
  distinguishable from an envelope that offered no suggestion. `Decision.Action`
  is an open string on purpose: the set of dispositions belongs to the
  interaction, and enumerating them here would make the wire format the
  bottleneck for every new one.

  `Registry.ValidateResponse` now rejects an answer with an empty `questionId`
  and a decision with an empty `itemId` or `action`, for every response kind.
  These are structural, not per-type — such an entry is unaddressable by any
  consumer whatever the envelope type.

- **`ResponseStatus.IsTerminal()` — the conflict contract.** Terminal statuses
  (`submitted`, `canceled`, `error`, and the legacy `cancelled` spelling) close
  the interaction; `partial` does not. An unrecognized status is not terminal,
  because an unknown state is not a resolution and treating it as one discards a
  response.

  This is the piece of response semantics that was previously left for every
  host to reinvent, and it has one specific failure mode worth naming: **a host
  that claims an envelope on a `partial` submission makes its own protocol
  unreachable.** The interaction can never be completed, because the completing
  submission collides with the draft that preceded it — `partial` becomes a
  state you can enter and never leave. Hosts with a single "record the response"
  path should branch on `IsTerminal` before taking it: record terminal responses
  immutably and answer a later submission with a conflict carrying the recorded
  response, and let a `partial` be replaced by the submission that follows it.

  The distinction matches the draft-versus-resolution split Tangent's retention
  ADR arrived at independently, where a participant draft and an immutable
  participant resolution are separate custody subjects.

  Internal lifecycle markers a host sets while dispatching (a "handling" claim,
  a "failed" outcome) are host instance state and deliberately not response
  statuses.

- README section documenting the typed channels, the conflict contract, and why
  the transport route is not part of it.

### Removed
- **BREAKING: the core manifest no longer carries `component`, `export` or
  `props`.** All three were removed from `manifest/envelopes.yaml` (17 of its 18
  entries carried them), from the manifest metaschema, and from
  `ManifestEntry`. Core envelope types now assert wire identity only.

  **Why deletion rather than repointing.** The `component` field named a
  filesystem path inside one specific host application's source tree — e.g.
  `components/chat/envelopes/primitives/InfoCard`. That is an appearance claim,
  and a wire contract library has no authority to make one: it owns type
  identity, payload schema, validation and compatibility, and nothing about how
  a payload looks. The paths were also simply wrong for every consumer other
  than the one host they were extracted from, since they are root-relative with
  no package root and resolve only against that host's tree. `export` named a
  React symbol at that path and `props` encoded which prop name that host's
  renderer feeds the payload to — the same claim in different words, with
  `props` being the one that encoded actual renderer behaviour.

  Repointing the field at a published design-system export was considered and
  rejected. It recreates the identical coupling one layer over, inside a library
  that must never assert appearance, and it would establish a second appearance
  authority beside the design system — leaving two token sets to reconcile
  later. The replacement for this field is a **binding** artifact, and a binding
  belongs outside this library.

  **Nothing was migrated because nothing consumed it.** The field's only
  machine-readable output was `ENVELOPE_IMPORT_METADATA` in the generated
  TypeScript, which has no import site anywhere in the portfolio — including in
  the host whose paths it encoded, which generates its UI types with its own
  script reading the schema directory directly.

  **The removal is enforced by name, not by omission.** The three properties are
  gone from `manifest/envelopes.schema.json` and `additionalProperties` is
  `false`, so a manifest reasserting any of them fails validation rather than
  being quietly accepted the next time a sweep regenerates the catalog.
  `TestParseManifest_carriesNoPresentationMetadata` and
  `TestManifest_metaschemaRejectsPresentationFieldsByName` pin both halves.

  The pre-deletion mapping for all 17 types is recorded in the CW-20260910-0113
  handoff table, including the non-1:1 row where `approval-card` and
  `subagent-spawn-approval` shared one component with two different prop-feeding
  paths.

  **Migration.** Consumers reading `ManifestEntry.Component`/`.Export`/`.Props`
  must drop those reads; the fields no longer exist. Consumers reading
  `TypeSpec.TypeScript.Import` or `TypeSpec.UIMetadata` keep compiling — both
  types are retained — but core types now leave them empty, so a core-only
  catalog generates an empty `ENVELOPE_IMPORT_METADATA`. Hosts that need a
  type-to-component mapping should own that mapping themselves.

- **Five app-specific JSON Schemas removed from the shared `manifest/schemas/`
  directory**, each named here so its absence is discoverable rather than
  mysterious:

  | Schema | Title | Disposition |
  |---|---|---|
  | `giphy-modal.schema.json` | Giphy Modal | dead — feature cut from its only host |
  | `kb-result.schema.json` | KB Result | dead — feature cut from its only host |
  | `resolution-capture.schema.json` | Resolution Capture | dead — feature cut from its only host |
  | `ticket-confirmation.schema.json` | Ticket Confirmation | dead — feature cut from its only host |
  | `ticket-form.schema.json` | Ticket Form | dead — feature cut from its only host |

  All five shipped in the shared schema directory with **no corresponding entry
  in `manifest/envelopes.yaml`**. They were seeded out of one application's
  source tree before the catalog was tightened and were never declared as core
  types, so `LoadCore` never registered them: they were reachable only as
  unregistered compatibility resources on the exported catalog.

  **Each was verified dead before deletion, not assumed.** The originating
  application cut every one of them in its own plugin-removal work — its
  `OrphanTypes` list is now an empty slice and its `RegisterOrphans` call is a
  no-op; the `kb-result` envelope builder was removed alongside the rest. No
  application in the portfolio emits, registers, or renders any of the five, and
  none of the five generated TypeScript data types has an import site anywhere.
  A schema with a live consumer would have been routed to its owning
  application rather than deleted.

  Every schema the module ships now corresponds to a declared core type — 18
  types, 18 schemas, set difference empty in both directions. That invariant is
  pinned by `TestExportCatalog_shipsNoUnregisteredSchemas`, so the next stray is
  a test failure rather than a discovery two years later.

  This resolves the "seed manifest carries five orphan schemas" known limitation
  recorded under 0.4.0.

  **Migration.** `-include-unregistered-schemas` is now a no-op against the core
  catalog, since there is nothing unregistered to include. The flag and the
  mechanism remain supported. Consumers that resolved any of the five schemas
  through the exported catalog will no longer find them; those types are not
  part of this library's contract and their owning application should ship them
  itself.

### Changed
- `codegen.TypeScript` no longer labels `ENVELOPE_IMPORT_METADATA` as
  "host-neutral". That claim was true of the generated data types, which derive
  from JSON Schema alone, and false of the import map, whose every entry names a
  path inside a particular host's tree. The emitted comment now says so, and the
  package documentation distinguishes the two outputs.
- Unknown manifest entry keys now reach `ManifestEntry.Extra` more completely:
  `component`, `export` and `props` are no longer stripped during decode, so a
  reintroduced key surfaces in `Extra` (and fails the metaschema) instead of
  vanishing silently.

### Retained deliberately
- `ImportMetadata`, `TypeSpec.UIMetadata` and the plugin-facing
  `PluginManifestEntry.UIMetadata` (`ui:`) all remain. A plugin declaring a
  component path for its own host is that host's decision, not this library
  asserting appearance, and `UIMetadata` is still the v0.3 compatibility view.
  Only the library's own core manifest stopped making the claim.

## [0.4.0] - 2026-09-04

### Added
- Module-owned build-time catalog export: `Registry.ExportCatalog` exposes the
  canonical YAML manifest, manifest schema, raw per-type JSON Schemas, typed
  TypeScript/import metadata, plugin registrations, and deterministic source
  identity. `cmd/envelopes-export` emits that catalog or generated TypeScript
  from the exact module version selected by a consumer's `go.mod`, removing the
  need for a sibling source checkout.
- Host-neutral TypeScript generation in `codegen.TypeScript`, including data
  interfaces/aliases, local `$defs`, `EnvelopeType`, `EnvelopeDataMap`, and
  structured component import metadata. Compatibility schemas that are shipped
  but not registered are an explicit opt-in.
- `SchemaDocument` and `SchemaMetadata` expose source JSON, standard
  annotations, common validation vocabulary, and uninterpreted custom
  annotations. Core and `RegisterTypeFromManifest` plugin schemas use the same
  surface.
- `ValidationError.Details` returns bounded structured leaf failures with
  instance/schema pointers, schema URI and keyword, expected values,
  actual-value summaries, and applicable schema metadata. Existing
  `errors.Is`/`errors.As` behavior and the wrapped
  `*jsonschema.ValidationError` remain intact.
- Consumer-style integration coverage runs a separate temporary Go module and
  proves runtime validation, plugin extension, catalog export, annotations, and
  TypeScript generation work outside the library checkout.

### Changed
- The cancellation vocabulary is coherent and US-English-first across the Go
  API, response wire values, the `session-task` schema, and generated
  TypeScript. New code uses `ResponseStatusCanceled` (`"canceled"`) and
  `ErrorCodeUserCanceled` (`"user-canceled"`). `ResponseStatus.IsCanonical`
  distinguishes new-output values, while `ResponseStatus.Canonical` and
  `CanonicalErrorCode` normalize persisted legacy input before re-emission.
- `TypeSpec` now carries `DataSchemaDocument`, `PayloadSchemaDocument`, and
  typed `TypeScript` metadata. The legacy `UIMetadata` map remains populated for
  v0.3 source compatibility. Registry lookup/export paths defensively copy
  mutable metadata.
- Unknown core-manifest entry keys are preserved as import metadata extras
  rather than dropped. The library preserves these hints but does not assign
  host-specific presentation semantics.
- Catalog source identity now distinguishes ordinary selected modules, local
  replacements, and versioned replacements without leaking local paths.
  Registry metadata is normalized into independently owned JSON values, schema
  documents are authoritative over simultaneously supplied compiled schemas,
  boolean schemas in generated properties/items/combinators/`$defs` and
  `not: true` generate their correct TypeScript shapes, open objects remain
  open, and `ValidationError.Error()` no longer includes the raw validator
  message.

### Deprecated
- `ResponseStatusCancelled` (`"cancelled"`) and
  `ErrorCodeUserCancelled` (`"user-cancelled"`) remain source- and wire-stable
  compatibility values throughout v0.4.x. The `session-task` schema and
  generated TypeScript likewise accept both `"canceled"` and legacy
  `"cancelled"` in this window. New emitters must use the US spelling. The
  British-spelled constants and accepted wire values are scheduled for removal
  in v0.5.0.

### Migration
- The v0.3.0 `ManifestEntry` and `ValidationError` value types were comparable;
  v0.4.0 adds map/slice fields, so they no longer satisfy Go's `comparable`
  constraint and cannot be compared with `==` or used as map keys. Key
  manifests by a stable scalar such as `entry.Type`, compare only the fields
  relevant to the caller, and inspect validation failures with `errors.Is`,
  `errors.As`, and `ValidationError.Details` instead of comparing
  `ValidationError` values.
- v0.4.0 adds exported fields to `ManifestEntry`, `ValidationError`, and
  `TypeSpec`, so external unkeyed composite literals for those types no longer
  compile. Convert positional literals to keyed literals, for example
  `envelopes.ManifestEntry{Type: "info-card", Component: "cards/InfoCard",
  Export: "InfoCard"}` and `envelopes.TypeSpec{Name: "plugin.card", Source:
  envelopes.TypeSourcePlugin}`. Keyed literals also remain source-compatible
  when later releases add fields.
- From v0.2.x, stop emitting `"cancelled"` / `"user-cancelled"`; migrate stored
  values when practical and use `ResponseStatus.Canonical` plus
  `CanonicalErrorCode` while reading unmigrated records. Existing Go callers
  keep compiling and their deprecated constants retain their exact historical
  values during v0.4.x. From v0.3.0, the `session-task` canonical spelling stays
  `"canceled"`; v0.4.0 only restores bounded read compatibility for v0.2 data.
- Replace scripts that open
  `../../libs/go-envelopes/manifest/{envelopes.yaml,schemas/}` with
  `go run github.com/hollis-labs/go-envelopes/cmd/envelopes-export -format
  catalog` (for host wrapper generators) or `-format typescript` (for the
  module-owned data types). Generated headers now identify their module version
  and manifest digest.
- Replace raw `EmbeddedFS` reads used only to recover annotations with
  `spec.DataSchemaDocument.Metadata()` or `MetadataAtInstancePath`. Keep
  host-specific wording and routing decisions in the host.
- Plugin hosts already using `RegisterTypeFromManifest` need no separate path:
  their schemas and import metadata now appear in `ExportCatalog` and
  `codegen.TypeScript`. Direct `RegisterType` callers should provide a
  `DataSchemaDocument` when build-time schema export is required.
- Direct `RegisterType` callers should account for metadata normalization:
  object, array, and numeric values returned by registry snapshots use
  `map[string]any`, `[]any`, and `json.Number`; structs and pointers become
  their decoded JSON value. Invalid JSON metadata now fails during
  `RegisterType`, before insertion, instead of surfacing during catalog export.

## [0.3.0] - 2026-08-24

### Changed
- **Breaking (wire value).** `session-task`'s `status` enum now spells the
  terminal caller-stopped state the US way: `cancelled` -> `canceled`. Nanite
  adopted US English as its project standard and this schema is the authority
  for that value — the host generates its TypeScript types from it, so the
  value cannot be migrated consumer-side. A payload emitting `"cancelled"` is
  now invalid; one emitting `"canceled"` is valid.

  Hosts that persist `session-task` payloads need a data migration for the
  stored value. Nanite's is `148_us_english_canceled_status.sql`. Note that
  validation is emit-path only in that host — `ValidateEnvelope` checks
  kind/version/registration and never the payload schema — so read paths there
  were unaffected; confirm the same before assuming it holds elsewhere.

### Known limitations
- **This release is deliberately half-migrated.** The Go status and error
  vocabulary keeps the British spelling: `ResponseStatusCancelled`
  (`"cancelled"`) and `ErrorCodeUserCancelled` (`"user-cancelled"`) in
  `types.go` are unchanged. Renaming them is source-breaking for consumers
  that use the identifiers, and `ResponseStatusCancelled`'s value is persisted
  by at least one of them, so it needs a coordinated change with its own data
  migration rather than a spelling sweep. Finishing it is a follow-on and will
  require another breaking bump.


## [0.2.0] - 2026-08-23

### Changed
- `subagent-spawn-approval`'s `component`/`export` UI-rendering-hint fields
  now point at `ApprovalCard` instead of a standalone
  `SubagentSpawnApprovalCard` — the consuming host folds the type's UI
  (risk badge, collapsible prompt/advanced sections, reason field) into a
  composed `ApprovalCard` flavor discriminated by the envelope's `type` at
  render time, per the "compose, don't multiply types" primitive-set
  principle. The manifest `type` itself is unchanged (still
  `subagent-spawn-approval`, schema unchanged) — only the two UI-hint
  fields moved. No Go API or validation-behavior change.

### Added
- `report-card` schema gained an optional `session_link` object
  (`{label, url}`) — a link back to the full session/task/run a report
  distills, so hosts can render "summary + link" instead of a raw
  transcript dump. Backward compatible: existing payloads without the
  field remain valid.
- `list-card` schema gained an optional top-level `data_source` object
  (`{kind, scope, scope_id, ...}`) and optional per-item `id`/`status`
  fields, so a host can compose a live, re-fetching checklist (e.g. a
  todo list) on top of the shared `list-card` primitive instead of
  minting its own top-level type for it. `status` (`pending`/`done`)
  renders a checkbox instead of a bullet/number; `data_source` signals
  the frontend to re-fetch `items` at render time rather than trusting
  the static array. Backward compatible: existing payloads without
  these fields remain valid.
- `confirmation-card` schema gained an optional top-level `data_source`
  object (`{kind: "plan_approval", plan_id}`) and an optional
  `request_changes_label`, so a host can compose a live plan-approval
  confirmation on top of the shared `confirmation-card` primitive
  instead of minting its own top-level type for it. `data_source`
  signals the frontend to re-fetch live status at render time rather
  than trusting only `prior_response`, and to route confirm/cancel
  through the source's own mutation. Backward compatible: existing
  payloads without these fields remain valid.
- `table-card` schema gained an optional `$defs/action` definition plus
  optional root-level `actions` (row-scoped) and column-level `actions`
  (column-scoped) properties, so interactive tables can render
  schema-validated button-group actions (`id`, `label`, `style`,
  `confirm`, `confirm_message`) instead of a host inventing its own ad
  hoc action shape. Backward compatible: existing payloads without
  `actions` remain valid.

### Removed
- `todo-list` core type dropped from the manifest. It never shipped a
  JSON Schema (validation passed on type-name alone) and its own
  frontend component always re-fetched via a live query rather than
  reading a data payload — a pure pointer/trigger shape. Rebuilt as a
  `list-card` composition (`data_source: {kind: "todos", scope,
  scope_id}` + per-item `status`) instead of staying a separate
  top-level type, per the "compose, don't multiply types" primitive-set
  principle.
- `plan-review` core type dropped from the manifest. Rebuilt as a
  `list-card` + `confirmation-card` composition instead of staying a
  separate top-level type: `list-card`'s schema gained an optional
  `data_source: {kind: "plans", plan_id}` (live step list, per-step
  toggle via the same mutation the standalone type used) and
  `confirmation-card`'s schema gained an optional `data_source: {kind:
  "plan_approval", plan_id}` plus `request_changes_label` (live
  approve/reject/request-changes via the same mutations). Per the
  "compose, don't multiply types" primitive-set principle.
- `question-form` core type dropped from the manifest, along with its
  orphaned `manifest/schemas/question-form.schema.json`. It predated the
  Cards primitive-composition model and was the only core type requiring
  host-side special-case persistence handling; cut alongside a host-side
  rebuild of structured multi-field user input as a composed primitive if
  the need resurfaces.
- `message-request`/`message-reply`/`message-notification`/`message-handoff`
  core types dropped from the manifest. Unfinished scaffolding for a
  messaging-subsystem Card surface that was never built out with frontend
  components or schemas, and redundant with the real `agent_messages.kind`
  DB enum consumers already use to classify these wire kinds. Neither type
  ever shipped a JSON Schema.
- `chat-loop-budget-soft-warning` core type dropped from the manifest,
  along with `manifest/schemas/chat-loop-budget-soft-warning.schema.json`.
  It signaled a chat loop crossing a soft `max_turns` budget without ever
  terminating the loop — a telemetry-only warning, not a real control.
  Cut alongside the consuming host's removal of the soft `max_turns`
  budget mechanism itself (nanite Phase 0 item 12).

## [0.1.1] - 2026-05-10

First public release. No public Go API changes vs `v0.1.0`; this is a
docs + hygiene pass to take the repository public.

### Added
- `examples/` directory with three runnable programs covering the
  primary API surfaces:
  - `examples/validate/` — `go run ./examples/validate`. Load the
    core registry, validate a known-good and a known-bad envelope.
  - `examples/plugin/` — `go run ./examples/plugin`. Register a
    plugin-owned envelope type at runtime via
    `RegisterTypeFromManifest`, then validate against it.
  - `examples/contract/` — `go test ./examples/contract`. Wire
    `envelopestest.RunContract` into a host's test suite.
- `.gitignore` entries for agent / AI scaffolding files so they cannot
  land accidentally in this public repo.

### Changed
- README rewritten for a public audience: status banner, godoc link,
  pointer to `examples/`, host-neutral framing.
- Manifest YAML comments and per-type JSON Schema `description` fields
  reframed to drop internal ticket IDs and host-specific terminology.
  Schema `description` text is metadata only — no validation behavior
  changed.
- Inline godoc comments in `registry.go`, `types.go`, and `manifest.go`
  reframed to describe behavior in host-neutral terms.

### Removed
- A host-specific internal migration recipe under `docs/` — not
  relevant to public consumers. Migration cookbooks for specific host
  applications now live with those applications.

## [0.1.0] - 2026-05-08

### Added
- Initial extraction of the envelope-machinery seed into a standalone
  shared library.
- Canonical YAML manifest at `manifest/envelopes.yaml` plus per-type
  JSON Schemas at `manifest/schemas/`. Both feed the TypeScript
  companion's codegen unchanged.
- Core registry (`Registry`) with `LoadCore`, `Lookup`, `Has`, `All`,
  `Names`, `Len`, and `Default` accessors.
- Validator with `ValidateEnvelope` and `ValidateResponse`, backed by
  `santhosh-tekuri/jsonschema/v6`.
- First-class plugin extension API: `RegisterType`, `UnregisterType`,
  `UnregisterPlugin`, `RegisterTypeFromManifest`. Plugin types use
  namespaced kebab-case names with one or more dot separators
  (`<plugin-id>.<type>` or `<vendor>.<plugin-id>.<type>`); un-namespaced
  names are reserved for core types.
- Typed errors: `ErrUnknownType`, `ErrSchemaValidation` (via
  `*ValidationError`), `ErrConflict`, `ErrInvalidName`,
  `ErrCoreTypeProtected`, `ErrUnsupportedKind`.
- Contract test helper at `envelopestest.RunContract` for downstream
  consumers.
- Documentation: README, manifest spec, extension API.

### Known limitations
- `Registry.ValidateResponse` does not yet enforce per-type response
  kind constraints (e.g. forcing `info-card` to `ack`). It validates
  base shape and any registered `PayloadSchema`. Tightening is a
  follow-on once the protocol spec codifies per-type response
  contracts.
- The seed manifest carries five orphan schemas (`kb-result`,
  `giphy-modal`, `resolution-capture`, `ticket-form`,
  `ticket-confirmation`) without YAML entries. They ship in
  `manifest/schemas/` for backward compatibility but are not loaded by
  `LoadCore`. Catalog cleanup is a separate task.
