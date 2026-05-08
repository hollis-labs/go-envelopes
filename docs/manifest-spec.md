# Manifest spec

`go-envelopes` is manifest-driven. The YAML in `manifest/envelopes.yaml`,
together with the per-type JSON Schemas in `manifest/schemas/`, is the
canonical source of truth. The same files feed `ts-envelopes` codegen
without modification.

## File layout

```
manifest/
  envelopes.yaml             # the manifest
  envelopes.schema.json      # JSON Schema that validates the manifest
  schemas/
    <type>.schema.json       # per-type data shape (one per registered type, optional)
```

## `envelopes.yaml`

```yaml
core:
  - type: info-card
    component: components/chat/envelopes/primitives/InfoCard
    export: InfoCard
    description: A simple informational card with variant styling.

  - type: approval-card
    component: components/chat/envelopes/ApprovalCard
    export: ApprovalCard
    props: approval

  - type: message-request
    # backend-only — no React component
```

### Per-entry fields

| Field | Required | Notes |
|---|---|---|
| `type` | yes | Kebab-case identifier; matches the `type` field on the wire. |
| `description` | no | Human/agent-facing description; surfaced through `TypeSpec.Description`. |
| `component` | no (TS-side) | Path to the React component, relative to `ui/src/` in the consumer. Ignored by Go. |
| `export` | no (TS-side, paired with `component`) | Named export from the component module. Ignored by Go. |
| `props` | no (TS-side) | Renderer-side prop discriminator (e.g. `approval`, `proposal`). Ignored by Go. |

The Go side reads only `type` and `description` directly. `component`,
`export`, and `props` flow through unchanged on `TypeSpec.UIMetadata` so
TS codegen and scaffolding tools can read them via `Registry.All()`.

### Adding a core type

1. Add an entry to `manifest/envelopes.yaml`.
2. (Optional) Drop a JSON Schema at `manifest/schemas/<type>.schema.json`
   that validates the envelope's `data` field.
3. Add tests covering a known-good and a known-bad payload.
4. Tag a minor release.

## Per-type JSON Schemas

Each entry in the YAML manifest MAY ship a JSON Schema at
`manifest/schemas/<type>.schema.json`. The schema validates the
envelope's `data` field. Entries without a schema register with a nil
`DataSchema`; `ValidateEnvelope` then verifies only that the type is
registered.

The schema document SHOULD include:

- `$schema` set to `https://json-schema.org/draft/2020-12/schema`
- `$id` matching the type name (for stable error references)
- `type: object`
- `additionalProperties: false` (recommended; keeps payloads tight)

Example (`info-card.schema.json`):

```json
{
  "$id": "info-card",
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Info Card",
  "type": "object",
  "required": ["title", "body"],
  "properties": {
    "title": {"type": "string"},
    "body":  {"type": "string"},
    "variant": {"type": "string", "enum": ["info","success","warning","danger"]}
  },
  "additionalProperties": false
}
```

### Custom annotation keywords

Schemas MAY carry annotations the JSON Schema compiler does not interpret.
The Nanite seed uses `default_render_target` (a panel ID) for runtime
routing; that keyword is not consumed by the registry — consumers that
care can read the raw schema document via `EmbeddedFS()` or by carrying
their own copy.

## Plugin-supplied manifests

Plugin types are NOT added to `envelopes.yaml`. They register at runtime
via the [extension API](extension-api.md). The same shape (`type`,
`description`, optional `responseKind`, optional `ui` map) is accepted by
`Registry.RegisterTypeFromManifest` so plugin manifests stay symmetrical
with core manifests.

## ts-envelopes parity

`ts-envelopes` reads the same YAML + JSON Schemas as its source of truth.
No Go-specific keys live in the manifest, and Go does not interpret
TS-specific keys. Adding a new TS-side rendering hint (e.g. CSS class
hint) is safe — Go ignores unknown YAML keys via the `Extra`-style
passthrough on `ManifestEntry`.
