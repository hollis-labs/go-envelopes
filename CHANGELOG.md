# Changelog

All notable changes to `go-envelopes` are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/) and the package
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
