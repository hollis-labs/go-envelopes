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

`TypeSpec.UIMetadata` carries TS-side rendering hints unchanged; Go
hosts can ignore it or surface it through their own UI tools.
