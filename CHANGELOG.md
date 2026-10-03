# Changelog

All notable changes to `go-envelopes` are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/) and the package
adheres to [Semantic Versioning](https://semver.org/).

## [0.5.0] - 2026-10-03

### Added

- Typed return channels on `Response`: optional `Answers []Answer` and
  `Decisions []Decision` alongside `Payload`, both with `omitempty`.
  `ValidateResponse` checks question IDs, item IDs and decision actions for every
  response kind. `ResponseStatus.IsTerminal` supplies the terminal-versus-draft
  distinction; the documented conflict contract leaves transport/storage to the
  host. README documents the typed channels, conflict contract and host-owned
  transport. ([8827ce4](https://github.com/hollis-labs/go-envelopes/commit/8827ce41ec220719cb59732f35c35e12308dc327))
- `TypeSupport`, `Registry.CheckSupport` and `Registry.SupportedTypes` let hosts
  declare supported types and named exclusions. `SupportGap` reports unclaimed,
  unknown or conflicting names. No catalog partition or host emission policy is
  imposed. ([91d9ea3](https://github.com/hollis-labs/go-envelopes/commit/91d9ea34e2b214cc99fc7d3fb4ebd5a3305b7ad5))
- Independent `admin` contract v1: app-scoped declarations, flat scalar settings
  and semantic validation, redacted snapshots, health/stat observations, and
  host-owned resolution/transaction contracts. No HTTP, storage or lifecycle
  implementation in the core.
  ([1fabfa3 / PR #4](https://github.com/hollis-labs/go-envelopes/commit/1fabfa3a783cacf326359ea38eb1a9efe412839a))
- Optional `admin/adminhttp` host-mounted handler: caller-filtered discovery,
  declared reads/observations and validate/update/reset commands, required host
  authorization/command protection, strict command decoding, strong ETag
  preconditions and staged atomic transaction orchestration. No listener,
  automatic retry, live apply or restart.
  ([59c1506 / PR #5](https://github.com/hollis-labs/go-envelopes/commit/59c150641cfab728f74621098c506279652d84e2))

### Changed

- **Breaking:** the core manifest asserts wire identity only. `component`,
  `export` and `props` are removed from the YAML, metaschema and public
  `ManifestEntry` fields. The metaschema refuses those keys; unknown decoded
  keys remain visible in `Extra`. Core catalog component hints are empty and
  core-only `ENVELOPE_IMPORT_METADATA` is empty. Generated data types remain
  schema-derived; import metadata is explicitly host-specific.
  ([f1593e4](https://github.com/hollis-labs/go-envelopes/commit/f1593e4e7a357dc728f17d5379e1e2d98e97d31e))
- Module Go directive raised from `1.26.1` to `1.26.6`.
  ([e507597](https://github.com/hollis-labs/go-envelopes/commit/e5075970a913bf3f50692bc0444053e3a90566ca))
- Legacy `"cancelled"` / `"user-cancelled"` compatibility and deprecated Go names
  are retained in v0.5.x. Removal is not scheduled and will be announced first;
  new output remains canonical US spelling. This supersedes the removal
  schedule in the historical v0.4.0 entries and published release notes.
  Live docs/comments and the session-task status description are corrected;
  the description change moves catalog identity/generated documentation without
  changing its enum or validation.
  ([b6ec75d / PR #7](https://github.com/hollis-labs/go-envelopes/commit/b6ec75d8211c56757699ed5fe645b4eb318510e7))

### Fixed

- Admin declaration revision is checked before group keys/types and effective
  permissions, then ETag. After structural decoding, stale revision plus an
  unknown key returns 409 `manifest_changed`; the unknown key with current
  revision returns 400. Both stale revision and stale ETag return 409; current
  revision plus stale ETag remains 412 `value_conflict`. Applies to validate
  and transactional update/reset; authorization and malformed-body checks
  remain earlier.
  ([83386cd / PR #6](https://github.com/hollis-labs/go-envelopes/commit/83386cd78c2ac5a1f5dbcf91508951b10cd3611e))

### Removed

- Unregistered compatibility schema resources `giphy-modal.schema.json`,
  `kb-result.schema.json`, `resolution-capture.schema.json`,
  `ticket-confirmation.schema.json` and `ticket-form.schema.json`. They had no
  core manifest entries and were never registered by `LoadCore`.
  `IncludeUnregisteredSchemas` and its command flag remain supported, but the
  core catalog has no unregistered schema resources to include.
  ([f1593e4](https://github.com/hollis-labs/go-envelopes/commit/f1593e4e7a357dc728f17d5379e1e2d98e97d31e))

### Migration

- Replace removed `ManifestEntry` field reads and own renderer bindings in the
  host; regenerate and review catalog/TypeScript output. `ImportMetadata`,
  `TypeSpec.TypeScript.Import`, `TypeSpec.UIMetadata` and plugin `ui:` metadata
  remain available. Stop resolving the removed schema resources.
  ([f1593e4](https://github.com/hollis-labs/go-envelopes/commit/f1593e4e7a357dc728f17d5379e1e2d98e97d31e))
- `Response.answers` and `.decisions` are optional additions, but Go reflection
  can expand a public JSON schema even when old payload bytes are unchanged.
  Tangent's session history output reflects those fields. Chrispian ACCEPTED
  ADR0011 on 2026-10-03 for this exact expansion, satisfying its schema-freeze
  policy before the separate digest refresh. Inspect reflected contracts as
  well as JSON payloads when upgrading.
  ([8827ce4](https://github.com/hollis-labs/go-envelopes/commit/8827ce41ec220719cb59732f35c35e12308dc327);
  [consumer evidence, Tangent #76](https://github.com/hollis-labs/tangent/pull/76))
- Module version, `Envelope.V` and `admin.ContractVersion` are independent.
  `ProtocolVersion` remains 1; the breaking manifest change requires the v0
  minor bump, not a wire-version bump. See [v0.5.0 release notes](docs/releases/v0.5.0.md).

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
