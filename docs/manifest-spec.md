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
    description: A simple informational card with variant styling.

  - type: approval-card

  - type: session-task
```

### Per-entry fields

| Field | Required | Notes |
|---|---|---|
| `type` | yes | Kebab-case identifier; matches the `type` field on the wire. |
| `description` | no | Human/agent-facing description; surfaced through `TypeSpec.Description`. |

That is the whole list. `additionalProperties` is `false`, so anything else is
rejected.

### Removed in v0.5.0: `component`, `export`, `props`

Through v0.4.x an entry could also carry `component` (a host component path),
`export` (its named export) and `props` (a host prop discriminator). All three
were removed.

The reason is the ownership line this library sits on. A wire contract owns type
identity, payload schema, validation and compatibility. It does not own
appearance. `component` asserted a filesystem path inside one specific host
application's source tree — a claim go-envelopes has no authority to make, and
one that was wrong for every other consumer. `export` and `props` were the same
assertion in different words: `props` encoded which prop name one host's renderer
feeds the payload to.

They are refused **by name** rather than by omission: the properties are gone
from `envelopes.schema.json` and `additionalProperties` is `false`, so a manifest
that reasserts one fails validation instead of being quietly accepted by a
regenerating sweep.

Which component renders a type is a **binding** concern, and a binding artifact
belongs outside this library. Putting it back here would create a second
appearance authority alongside the design system.

Plugins are unaffected: `PluginManifestEntry` still accepts a `ui` map, because a
plugin declaring a component for its own host is that host's decision rather than
this library's assertion.

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
Schemas as runtime validation. No Go-only schema fork exists. Unknown entry keys
are preserved in `ImportMetadata.Extra` and the generator catalog, which lets
metadata evolve without teaching every consumer how to locate and re-parse the
YAML.

`TypeSpec.TypeScript.Import` still exists and is still populated for
plugin-registered types from their `ui` map. Core types leave it empty, so a
core-only catalog generates an empty `ENVELOPE_IMPORT_METADATA`. That is the
expected result, not a generation failure.

The canonical cancellation status is `"canceled"`. The `session-task` schema
also recognizes legacy `"cancelled"` through v0.4.x so old persisted payloads
remain readable; its description and enum order identify `"canceled"` as the
new-output value. The compatibility value is scheduled for removal in v0.5.0.

For module-resolved export and host-neutral TypeScript generation, see
[`generation.md`](generation.md).
