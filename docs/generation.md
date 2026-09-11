# Catalog and generation API

The released Go module owns the complete build-time contract. A consumer does
not need to locate `manifest/` in the module cache, clone this repository next
to its own checkout, or maintain another JSON-Schema-to-TypeScript walker.

## Command

Add a normal `require` for `github.com/hollis-labs/go-envelopes` to the
consumer's `go.mod`. These commands then run the generator from the version
selected by Minimal Version Selection:

```sh
go run github.com/hollis-labs/go-envelopes/cmd/envelopes-export \
  -format catalog -output build/envelope-catalog.json

go run github.com/hollis-labs/go-envelopes/cmd/envelopes-export \
  -format typescript -output ui/src/generated/envelope-types.ts
```

Use `-output -` (the default) for stdout. `-format catalog` is the default.
`-include-unregistered-schemas` on TypeScript output includes compatibility
schema resources that ship in the module but are intentionally absent from the
live registry. New consumers should normally omit that flag.

**As of v0.5.0 the module ships none**, so the flag is currently a no-op: every
schema in `manifest/schemas/` corresponds to a type declared in
`envelopes.yaml`. Five app-specific schemas (`giphy-modal`, `kb-result`,
`resolution-capture`, `ticket-confirmation`, `ticket-form`) previously shipped
without a declaration; they were removed in v0.5.0. The flag and the mechanism
remain supported for a future deliberate compatibility schema.

For a hermetic tool invocation independent of a consumer `go.mod`, suffix the
package with an exact released version:

```sh
go run github.com/hollis-labs/go-envelopes/cmd/envelopes-export@vX.Y.Z \
  -format catalog
```

## Catalog JSON

`Registry.ExportCatalog` returns the same `Catalog` value the command encodes.
Its stable top-level fields are:

- `formatVersion` — version of the catalog JSON shape.
- `source` — module path/version, wire protocol version, and SHA-256 manifest
  identity.
- `catalogDigest` — identity of this complete catalog, including plugin types.
- `manifestYAML` and `manifestSchema` — the canonical module-owned manifest
  sources.
- `types` — every registered core and plugin type, with response, schema,
  annotation, and TypeScript/import metadata.
- `schemas` — raw JSON Schema documents. `registered=false` marks a shipped
  compatibility resource that is not a live envelope type.

Arrays are sorted, maps use Go's deterministic JSON key ordering, and an
unchanged registry produces byte-identical JSON when encoded with the same
encoder settings. Registration rejects caller-supplied extension metadata that
is not JSON-representable. That validation occurs during `RegisterType`, before
the type is inserted, rather than later during `ExportCatalog`. Registered
objects/arrays/numbers are exposed as `map[string]any`, `[]any`, and
`json.Number`; structs and pointers become their corresponding decoded JSON
value. `ExportCatalog` never substitutes an empty digest for a failed snapshot
encoding.

`source.moduleVersion` identifies the code that actually supplied the catalog,
not merely the version requested on the left side of a `replace` directive. An
ordinary selected dependency reports its selected version. A local replacement
reports `(devel; local replacement)` without exposing its filesystem path. A
versioned replacement reports the replacement module path and version. A direct
unversioned build reports `(devel)`. Generated TypeScript includes this identity
and `source.manifestDigest` in its header, so checked-in output identifies what
generated it.

## Go API

```go
registry, err := envelopes.LoadCore(ctx)
if err != nil {
    return err
}

catalog, err := registry.ExportCatalog()
if err != nil {
    return err
}
generated, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
if err != nil {
    return err
}
```

The TypeScript generator emits:

- one exported data interface or alias per registered type;
- local `$defs` as collision-resistant, type-prefixed declarations;
- `EnvelopeType` and `EnvelopeDataMap`;
- `ENVELOPE_IMPORT_METADATA` with component path, named export, props hint,
  source, and plugin id — **populated by plugins only**. Core envelope types
  contribute no entries, so a core-only catalog emits an empty map. That is the
  expected result: the core manifest asserts wire identity and no appearance.

Boolean schemas are honored at the root and in recursively generated
properties, items, combinators, and `$defs`. The `not: true` applicator is
recognized as an always-failing schema and emits `never`; `not: false` adds no
constraint.

It does not emit framework imports or component-loader code. A React host can
turn plugin-supplied metadata into `lazy()` imports; another host can use an
entirely different loader. Host-specific component overrides and presentation
wording belong in that host.

**"Host-neutral" applies to the data types, not to the import map.** The data
interfaces, `EnvelopeType` and `EnvelopeDataMap` derive from JSON Schema alone
and are portable to any consumer. `ENVELOPE_IMPORT_METADATA` is not portable by
construction: every entry names a path and symbol inside one particular host's
source tree, and choosing your own loader does not make a foreign path resolve.
Through v0.4.x the core manifest filled this map with 17 entries pointing into a
single host application, under a comment reading "host-neutral". Both the entries
and the comment were removed in v0.5.0.

## Migrating from a sibling checkout

Before:

```sh
node scripts/generate-types.mjs --schema-dir ../../libs/go-envelopes/manifest/schemas
```

After:

```sh
go run github.com/hollis-labs/go-envelopes/cmd/envelopes-export \
  -format typescript -output ui/src/generated/envelope-types.ts
```

If a host needs its own wrapper code, consume `-format catalog` instead. The
catalog already contains parsed import metadata and raw schemas, so the wrapper
generator only implements host integration; it does not rediscover module
paths or traverse JSON Schema to produce data types.

## Plugin types

Plugin types enter generation through the same registry contract used for
runtime validation:

```go
if err := registry.RegisterTypeFromManifest(manifest, schema, "calendar"); err != nil {
    return err
}

catalog, err := registry.ExportCatalog() // now includes calendar.*
if err != nil {
    return err
}
generated, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
```

`RegisterTypeFromManifest` retains the source schema as a `SchemaDocument`,
compiles it for validation, records typed import metadata, and includes it in
the exported catalog. Advanced callers using `RegisterType` directly can set
`TypeSpec.DataSchemaDocument`. A supplied document is authoritative and the
registry always compiles it, replacing any simultaneously supplied compiled
`DataSchema`; `PayloadSchemaDocument` follows the same rule. This prevents
runtime validation from diverging from exported schema source. A direct
registration that supplies only an already-compiled schema still validates for
backward compatibility, but cannot export source bytes or annotations that were
never provided.

## Schema metadata and validation details

`TypeSpec.DataSchemaDocument.Metadata` returns root metadata.
`MetadataAtInstancePath` follows object properties, array items, and local
`$ref` values. Standard annotations (`title`, `description`, `default`,
`examples`, read/write/deprecation flags), common validation vocabulary
(`type`, `enum`, `required`, property names, `additionalProperties`), and
unknown extension annotations are exposed in typed fields.

Custom annotations are preserved in `SchemaMetadata.Custom`; the library does
not interpret them. For example, a host may read a routing hint and decide what
it means locally without putting that presentation policy into go-envelopes.

`ValidationError.Details` returns one `ValidationFailure` per leaf failure with
instance path, schema path and URI, keyword, schema-owned expected value, a
bounded actual-value summary, and the applicable schema metadata. Arbitrary
string/object payload contents are not copied into `Actual`; only type/length,
booleans, JSON numbers, and validation counts are retained. The legacy wrapped
`*jsonschema.ValidationError` remains reachable through `errors.As`, but that
raw underlying error may contain rejected payload values and must be treated as
sensitive. `ValidationError.Error()` and `Details()` are the bounded surfaces
intended for ordinary logs and diagnostics.

## Cancellation compatibility in generated types

US English is canonical for new wire data: `"canceled"` and
`"user-canceled"`. The `session-task` schema and generated TypeScript also
include the legacy `"cancelled"` status during v0.4.x so persisted v0.2 data
remains representable. The property description marks `"canceled"` as the
value to emit. The legacy schema value is scheduled for removal in v0.5.0.

Go response values follow the same window. New code uses
`ResponseStatusCanceled` and `ErrorCodeUserCanceled`; readers can call
`ResponseStatus.Canonical` and `CanonicalErrorCode` before re-emitting legacy
data. The British-spelled Go constants remain deprecated compatibility values,
not the canonical contract.
