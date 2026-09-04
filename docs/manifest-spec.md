# Manifest spec

`go-envelopes` is manifest-driven. The YAML in `manifest/envelopes.yaml`,
together with the per-type JSON Schemas in `manifest/schemas/`, is the
canonical source of truth. The module-owned `codegen.TypeScript` generator and
`cmd/envelopes-export` command consume the same embedded files.

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

  - type: session-task
    # backend-only — no React component
```

### Per-entry fields

| Field | Required | Notes |
|---|---|---|
| `type` | yes | Kebab-case identifier; matches the `type` field on the wire. |
| `description` | no | Human/agent-facing description; surfaced through `TypeSpec.Description`. |
| `component` | no (paired with `export`) | Host component path, preserved in `TypeSpec.TypeScript.Import.Component` and the exported catalog. |
| `export` | no (paired with `component`) | Named export from the component module, preserved in `TypeSpec.TypeScript.Import.Export` and the exported catalog. |
| `props` | no | Host-defined prop discriminator (e.g. `approval`, `proposal`), preserved in `TypeSpec.TypeScript.Import.Props` and the exported catalog. |

The Go manifest loader reads and preserves every field above. It maps
`component`, `export`, and `props` into the typed
`TypeSpec.TypeScript.Import` metadata used by `Registry.ExportCatalog` and the
module-owned `codegen.TypeScript` generator; `TypeSpec.UIMetadata` remains a
compatibility view for v0.3 consumers. Runtime validation does not use these
fields, and the library does not assign presentation or component-loading
semantics to them. Hosts make those policy decisions.

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
The seed manifest uses `default_render_target` (a panel ID) for host-side
routing hints. The registry preserves unknown keywords without interpreting
them: consumers can read the value from
`TypeSpec.DataSchemaDocument.Metadata().Custom`. This keeps host presentation
policy outside the library while avoiding filesystem access and duplicate JSON
parsing. Raw JSON remains available through `SchemaDocument.JSON()` and the
exported `Catalog.Schemas` list.

## Plugin-supplied manifests

Plugin types are NOT added to `envelopes.yaml`. They register at runtime
via the [extension API](extension-api.md). The same shape (`type`,
`description`, optional `responseKind`, optional `ui` map) is accepted by
`Registry.RegisterTypeFromManifest` so plugin manifests stay symmetrical
with core manifests.

## TypeScript parity

`codegen.TypeScript` reads the exported catalog built from the same YAML + JSON
Schemas as runtime validation. No Go-only schema fork exists. The known
`component`/`export`/`props` keys are available on
`TypeSpec.TypeScript.Import`; unknown entry keys are preserved in
`ImportMetadata.Extra` and the generator catalog. This lets metadata evolve
without teaching every consumer how to locate and re-parse the YAML.

The canonical cancellation status is `"canceled"`. The `session-task` schema
also recognizes legacy `"cancelled"` through v0.4.x so old persisted payloads
remain readable; its description and enum order identify `"canceled"` as the
new-output value. The compatibility value is scheduled for removal in v0.5.0.

For module-resolved export and host-neutral TypeScript generation, see
[`generation.md`](generation.md).
