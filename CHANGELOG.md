# Changelog

All notable changes to `go-envelopes` are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/) and the package
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-05-08

### Added
- Initial extraction from Nanite's internal envelope machinery.
- Canonical YAML manifest at `manifest/envelopes.yaml` plus per-type
  JSON Schemas at `manifest/schemas/`. Both feed `ts-envelopes` codegen
  unchanged.
- Core registry (`Registry`) with `LoadCore`, `Lookup`, `Has`, `All`,
  `Names`, `Len`, and `Default` accessors.
- Validator with `ValidateEnvelope` and `ValidateResponse`, backed by
  `santhosh-tekuri/jsonschema/v6`.
- First-class plugin extension API: `RegisterType`, `UnregisterType`,
  `UnregisterPlugin`, `RegisterTypeFromManifest`. Plugin types use
  namespaced names (`<plugin-id>.<type>`); core types are reserved.
- Typed errors: `ErrUnknownType`, `ErrSchemaValidation` (via
  `*ValidationError`), `ErrConflict`, `ErrInvalidName`,
  `ErrCoreTypeProtected`, `ErrUnsupportedKind`.
- Contract test helper at `envelopestest.RunContract` for downstream
  consumers.
- Documentation: README, manifest spec, extension API, Nanite migration
  recipe.

### Known limitations
- `Registry.ValidateResponse` does not yet enforce per-type response
  kind constraints (e.g. forcing `info-card` to `ack`). It validates
  base shape and any registered `PayloadSchema`. Tightening is a
  follow-on once protocol-spec-v1 codifies per-type response contracts.
- The seed manifest carries five orphan schemas (`kb-result`,
  `giphy-modal`, `resolution-capture`, `ticket-form`,
  `ticket-confirmation`) without YAML entries. They ship in
  `manifest/schemas/` for backward compatibility but are not loaded by
  `LoadCore`. Catalog cleanup is a separate task.
