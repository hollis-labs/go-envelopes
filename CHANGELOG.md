# Changelog

All notable changes to `go-envelopes` are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/) and the package
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `report-card` schema gained an optional `session_link` object
  (`{label, url}`) — a link back to the full session/task/run a report
  distills, so hosts can render "summary + link" instead of a raw
  transcript dump. Backward compatible: existing payloads without the
  field remain valid.

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
