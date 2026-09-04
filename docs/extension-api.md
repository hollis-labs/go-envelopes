# Plugin extension API

`go-envelopes` ships a core catalog of envelope types and exposes a
runtime API for plugin-SDK consumers to register additional types
alongside it. This is a first-class concern, not a retrofit — any host
embedding a Hollis Labs plugin runtime (or its own equivalent) can
extend the catalog at runtime.

## Surface

```go
func (r *Registry) RegisterType(spec TypeSpec) error
func (r *Registry) UnregisterType(name string) error
func (r *Registry) UnregisterPlugin(pluginID string) int
func (r *Registry) RegisterTypeFromManifest(manifestBytes, schemaBytes []byte, pluginID string) error
```

## Naming rules

Plugin types MUST use a namespaced name of the form
`<plugin-id>.<type>`. Both segments must be kebab-case-lower
(`[a-z0-9][a-z0-9-]*`).

| Form | Verdict |
|---|---|
| `myplugin.calendar-pick` | ✅ valid |
| `calendar-pick` | ❌ unnamespaced (rejected; reserved for core) |
| `MyPlugin.Calendar` | ❌ uppercase |
| `.calendar` / `myplugin.` | ❌ empty segment |

Attempts that fail the namespace check return `ErrInvalidName`.

## Conflict resolution

The first registration wins. A second `RegisterType` for an already-
registered name returns `ErrConflict`. The library does not silently
overwrite — the surrounding plugin host decides how to recover (log and
skip, prompt the user, prefer the newer plugin, etc.).

## Core type protection

`UnregisterType` refuses to remove types whose `Source` is
`TypeSourceCore`, returning `ErrCoreTypeProtected`. Plugin hosts cannot
strip core types out from under other consumers.

## Plugin lifecycle

The library does not know about plugin-SDK lifecycle hooks directly.
Plugin hosts hook their own load/unload events to call `RegisterType` and
`UnregisterPlugin`:

```go
host.OnPluginLoad(func(p *Plugin) {
    for _, decl := range p.Manifest.Envelopes {
        if err := registry.RegisterTypeFromManifest(decl.Manifest, decl.Schema, p.ID); err != nil {
            host.Log("envelope registration failed", "plugin", p.ID, "err", err)
        }
    }
})

host.OnPluginUnload(func(p *Plugin) {
    n := registry.UnregisterPlugin(p.ID)
    host.Log("envelopes dropped", "plugin", p.ID, "count", n)
})
```

`UnregisterPlugin` is the bulk path: it sweeps every plugin-owned type
whose `PluginID` matches and returns the count removed. Core types stay.

## Trust boundary

`go-envelopes` does NOT enforce plugin trust:

- It does not verify signatures.
- It does not gate capabilities.
- It does not isolate compiled schemas.

Trust evaluation is the surrounding plugin SDK's job. Hosts that load
untrusted plugin code should run that code in an isolated process, gate
capabilities at the manifest layer, and only then pass the registration
through to `RegisterType`. Once a registration reaches the registry, the
host has already decided to trust it.

## Versioning

Plugin types carry their own `Version` on `TypeSpec.Version`. The
library's semver does NOT track plugin types — a plugin can ship its
type at v2 while `go-envelopes` stays at v1. Hosts that need to gate
plugin-type versions should inspect `Lookup(name).Version` and act
accordingly.

## Discovery

Hosts that want to advertise the registered catalog (for agent capability
discovery) iterate `Registry.All()`:

```go
for _, spec := range registry.All() {
    fmt.Printf("%s\t(%s)\t%s\n", spec.Name, spec.Source, spec.Description)
}
```

`TypeSpec.UIMetadata` carries the same TS-side rendering hints semantically; Go
hosts can ignore it or surface it through their own UI tools. New code should
prefer the typed `TypeSpec.TypeScript.Import`; `UIMetadata` remains available
for v0.3 source compatibility.

`RegisterType` establishes an ownership boundary by encoding `UIMetadata` and
`TypeScript.Import.Extra` as JSON and decoding them into canonical Go shapes:
objects become `map[string]any`, arrays become `[]any`, numbers become
`json.Number`, and structs or pointers become their corresponding decoded JSON
value. Consequently, a caller that registers `[]string`, `map[string]string`,
or a struct pointer must not expect that same concrete Go type from `Lookup` or
`All`; type-assert against the canonical shape instead. Every returned snapshot
is independently owned.

Non-JSON values and failing `MarshalJSON` implementations now cause
`RegisterType` itself to return an error; failure is not deferred until
`ExportCatalog`. Normalization, including any caller-defined `MarshalJSON`
method, completes before the registry write lock is acquired. The type becomes
visible atomically only after normalization succeeds, and a marshaler may safely
query the registry while it runs.

## Build-time export and schema annotations

Plugin registrations made with `RegisterTypeFromManifest` participate in the
same catalog and TypeScript generation surface as core types:

```go
if err := registry.RegisterTypeFromManifest(manifest, schema, pluginID); err != nil {
    return err
}
catalog, err := registry.ExportCatalog()
if err != nil {
    return err
}
generated, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
```

The resulting `CatalogType` has `source: "plugin"`, `pluginId`, typed import
metadata, and a `schemaURI` linked to its `SchemaResource`. Unloading the plugin
removes both the registry type and its exported schema resource.

`TypeSpec.DataSchemaDocument` retains the plugin's JSON Schema source and
exposes `Metadata`/`MetadataAtInstancePath`. Unknown schema annotations are
preserved under `SchemaMetadata.Custom` without assigning host-specific
semantics. Validation failures for plugin types are returned as the same
`ValidationError.Details()` records used by core types.

Advanced callers using `RegisterType` directly may provide a
`DataSchemaDocument` created by `NewSchemaDocument`. The document is
authoritative: `RegisterType` always compiles it and replaces any simultaneously
supplied `DataSchema`. `PayloadSchemaDocument` applies the same rule to response
payload validation. Supplying only an already-compiled `DataSchema` remains
supported, but its unavailable source JSON and annotations cannot appear in
exported artifacts.
