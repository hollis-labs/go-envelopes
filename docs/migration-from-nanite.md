# Migrating from Nanite's internal envelope code

Nanite seeded `go-envelopes` and is the first consumer to migrate. This
recipe captures the steps so future apps doing similar extractions can
follow the same arc.

## Before

Nanite carried envelope machinery in three places:

- `nanite/config/envelopes.yaml` + `config/envelopes.schema.json` — the
  manifest.
- `nanite/internal/envelope/schemas/*.schema.json` — per-type JSON
  Schemas.
- `nanite/internal/chat/envelope.go` — runtime parser, validator,
  registry of type names.
- `nanite/internal/envelope/validator.go` — richer JSON Schema
  validation with structured error flattening for LLM hints.
- `nanite/internal/plugin/envelope_validator.go` — plugin-side schema
  registration + validation.

## After

- `manifest/` lives at the root of `go-envelopes`. Nanite's copies are
  deleted.
- `nanite/internal/chat/envelope.go` retains its fenced-block parser
  and Nanite-specific runtime fields (`target`, `render_target`,
  `mode`, etc.) but drops type-name registration in favor of
  `*envelopes.Registry`.
- `nanite/internal/envelope/validator.go` keeps the structured-error
  flattening (it's tuned for LLM hints) but reads compiled schemas from
  `envelopes.Lookup(...).DataSchema` instead of its own embed.
- `nanite/internal/plugin/envelope_validator.go` becomes a thin wrapper
  around `Registry.RegisterTypeFromManifest` and
  `Registry.UnregisterPlugin`.

## Step-by-step

1. **Add the dependency.** In Nanite's `go.mod`:
   ```
   require github.com/hollis-labs/go-envelopes v0.1.0
   replace github.com/hollis-labs/go-envelopes => ../go-envelopes
   ```
   Match the path-relative replace pattern Nanite already uses for
   `go-providers`, `go-runner`, etc.

2. **Construct the shared registry at startup.** Pick the place where
   Nanite already calls `chat.InitCoreTypes` and replace it:
   ```go
   reg, err := envelopes.LoadCore(ctx)
   if err != nil { /* fatal */ }
   chat.SetEnvelopeRegistry(reg)            // wire into the parser
   plugin.SetEnvelopeRegistry(reg)          // wire into plugin host
   ```

3. **Replace `chat.InitCoreTypes` + `chat.RegisterEnvelopeType`** with
   delegation to the shared registry:
   ```go
   func ValidateEnvelope(env Envelope, raw string) *EnvelopeError {
       if env.Kind == "" { /* missing_kind */ }
       if env.Version < 1 { /* missing_version */ }
       if env.Type != "" && !envelopeRegistry.Has(env.Type) {
           return &EnvelopeError{Raw: raw, Reason: "unregistered_type"}
       }
       return nil
   }
   ```
   The wire-format dance (`kind`/`version` fields, fenced blocks) stays
   in Nanite — `go-envelopes` is concerned with the typed `Envelope`
   shape, not the legacy chat-stream wrapping.

4. **Replace per-type schema loading** in
   `nanite/internal/envelope/validator.go`. Drop the embed FS and
   `loadSchema` cache; pull compiled schemas from the registry:
   ```go
   func ValidateData(envelopeType string, data any) error {
       spec, ok := envelopeRegistry.Lookup(envelopeType)
       if !ok {
           return fmt.Errorf("no schema registered for envelope type %q", envelopeType)
       }
       if spec.DataSchema == nil { return nil }
       if err := spec.DataSchema.Validate(data); err != nil {
           return &ValidationError{Type: envelopeType, Inner: err}
       }
       return nil
   }
   ```
   `FlattenSchemaError` and `StructuredError` stay — they're
   Nanite-shaped LLM hints, not lib concerns.

5. **Migrate plugin schema registration** in
   `nanite/internal/plugin/envelope_validator.go`. Replace
   `Host.RegisterPluginEnvelopeSchema` with a forwarder:
   ```go
   func (h *Host) RegisterPluginEnvelopeSchema(pluginID, envType string, schemaBytes []byte) error {
       manifest, _ := json.Marshal(map[string]any{"type": envType})
       return h.envelopeRegistry.RegisterTypeFromManifest(manifest, schemaBytes, pluginID)
   }
   ```
   `ValidatePluginEnvelope` is now `envelopeRegistry.ValidateEnvelope`
   (with a Nanite-shape→`envelopes.Envelope` adapter at the boundary).
   Plugin unload calls `envelopeRegistry.UnregisterPlugin(pluginID)`.

6. **Wire the TS codegen.** Nanite's `generate-envelopes` Make target
   currently reads from `nanite/config/envelopes.yaml`. Two options:

   - **A.** Add a build step that copies `../go-envelopes/manifest/`
     into `nanite/.go-envelopes-cache/` for the codegen.
   - **B.** Pass `--manifest-dir ../go-envelopes/manifest/` to the
     codegen script directly.

   Pick whichever is simpler; document it in the migration commit.

7. **Handle orphan schema files.** The seed extraction pulled in
   `kb-result`, `giphy-modal`, `resolution-capture`, `ticket-form`,
   `ticket-confirmation` — five JSON Schemas that exist in
   `manifest/schemas/` but have no entry in `envelopes.yaml`. These were
   already orphans in Nanite. Nanite's runtime referenced them through
   ad-hoc `ValidateData("kb-result", ...)` calls. Two paths:

   - Add manifest entries for the ones still in active use (likely
     `kb-result` for `BuildKBEnvelope`).
   - Have Nanite explicitly register the rest at startup via
     `RegisterTypeFromManifest` from the orphan schema files, with
     `pluginID = "nanite-legacy"`. Plan to migrate them out as part of
     a catalog-cleanup follow-on.

8. **Delete the seed.** Once the test suite passes, remove:
   - `nanite/config/envelopes.yaml`
   - `nanite/config/envelopes.schema.json`
   - `nanite/internal/envelope/schemas/`
   The `embed.FS` declaration in `internal/envelope/validator.go` goes
   away with the schemas dir.

9. **Run `make test` in Nanite.** Existing fixtures should still pass —
   the registry serves identical compiled schemas. Investigate any
   regression before declaring the migration done.

10. **Update `composition-map.md`.** In `agent-workspaces`,
    `knowledge/portfolio/composition-map.md` — flip
    `go-envelopes` from "proposed" to "shipped" under "Shared SDKs /
    libraries", and add the Nanite → go-envelopes consumer edge.

## Backward compatibility

The wire format is unchanged. Plugins that register envelope types via
Nanite's existing seams continue to work; under the hood, those calls
route to `Registry.RegisterTypeFromManifest`. No plugin redeploy is
required.

## Tangent (next consumer)

Tangent's Wails backend constructs its own `*Registry` via `LoadCore`
on startup and surfaces it to the React UI through MCP/WebSocket. The
core catalog is identical to Nanite's because both load the same
manifest. Tangent-specific envelope types (workflow runners, app-shell
chrome) register through the same extension API.
