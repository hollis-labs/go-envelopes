# go-envelopes

`go-envelopes` is the shared Go primitive library for the **Envelope UI
Protocol** — a small wire format for typed, host-rendered payloads that
agents send to host applications. The package ships a manifest-driven
type registry, JSON-Schema validator, and plugin extension API for
runtime registration of additional envelope types. The same module also owns
the build-time catalog and TypeScript generator, so consumers do not need a
sibling checkout of this repository.

It is transport-agnostic. It does not bind to MCP, SSE, or any other
transport — those concerns live in the host application. It also does
not persist envelope instances; storage is host-defined.

## Status

`v0.4.x` — pre-1.0. Public API may shift between minor versions; see
the CHANGELOG for breaking changes. The wire-format major version
(`Envelope.V`) is independent of the library version.

## Install

```sh
go get github.com/hollis-labs/go-envelopes
```

Godoc: <https://pkg.go.dev/github.com/hollis-labs/go-envelopes>

## Quickstart

```go
package main

import (
	"context"
	"log"

	envelopes "github.com/hollis-labs/go-envelopes"
)

func main() {
	reg, err := envelopes.LoadCore(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	env := &envelopes.Envelope{
		V:    envelopes.ProtocolVersion,
		ID:   "env_demo_1",
		Type: "info-card",
		Data: map[string]any{
			"title": "Hello",
			"body":  "world",
		},
	}
	if err := reg.ValidateEnvelope(env); err != nil {
		log.Fatal(err)
	}
}
```

More runnable examples live under [`examples/`](examples/):

- `examples/validate` — load the core registry and validate a known-good
  envelope. Run with `go run ./examples/validate`.
- `examples/plugin` — register a plugin-owned envelope type at runtime.
  Run with `go run ./examples/plugin`.
- `examples/contract` — wire `envelopestest.RunContract` into a host's
  test suite. Run with `go test ./examples/contract`.

## Build-time generation

Pin this module in the consuming application's `go.mod`, then run the command
from that application:

```sh
go run github.com/hollis-labs/go-envelopes/cmd/envelopes-export \
  -format catalog -output envelope-catalog.json

go run github.com/hollis-labs/go-envelopes/cmd/envelopes-export \
  -format typescript -output envelope-types.generated.ts
```

Both outputs state the selected module version, protocol version, and manifest
digest. The catalog contains the embedded YAML manifest, manifest schema,
per-type JSON Schemas, annotations, and component import metadata. The
TypeScript output is host-neutral: it emits data types and import metadata but
does not prescribe React, a loader, or presentation wording.

Go-based generators and plugin hosts can use the same surface directly:

```go
catalog, err := registry.ExportCatalog()
if err != nil {
    return err
}
source, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
```

See [`docs/generation.md`](docs/generation.md) for migration and plugin examples.

## Structured validation

Schema failures remain compatible with `errors.Is(err,
envelopes.ErrSchemaValidation)` and now expose bounded structured details:

```go
var validationErr *envelopes.ValidationError
if errors.As(err, &validationErr) {
    for _, failure := range validationErr.Details() {
        log.Printf("instance=%s schema=%s keyword=%s expected=%v actual=%v",
            failure.InstancePath, failure.SchemaPath, failure.Keyword,
            failure.Expected, failure.Actual)
    }
}
```

`TypeSpec.DataSchemaDocument` exposes parsed `SchemaMetadata`, including custom
annotation keywords, without requiring consumers to reopen embedded files.
Hosts decide how those facts are worded or presented to users.

`ValidationError.Error()` and `Details()` are safe bounded diagnostic surfaces.
The raw validator error remains available through `errors.As` for compatibility,
but may contain rejected payload values and should not be logged.

## Cancellation vocabulary

The canonical wire spelling is US English: `"canceled"` for response and
`session-task` statuses, and `"user-canceled"` for the protocol error code.
Use `ResponseStatusCanceled` and `ErrorCodeUserCanceled` for new output.

For a bounded migration window, v0.4.x continues to read the v0.2-era
`"cancelled"` status and `"user-cancelled"` error code. The deprecated
`ResponseStatusCancelled` and `ErrorCodeUserCancelled` constants keep their
historical values so existing emitters do not silently change wire behavior on
upgrade. Normalize stored input before re-emitting it:

```go
response.Status = response.Status.Canonical()
if response.Error != nil {
    response.Error.Code = envelopes.CanonicalErrorCode(response.Error.Code)
}
```

The compatibility spellings and deprecated Go names are scheduled for removal
in v0.5.0. New payloads should not depend on the v0.4.x exception.

## Layout

- `manifest/` — canonical YAML manifest + per-type JSON Schemas (single source of truth, language-agnostic).
- `*.go` (root package `envelopes`) — registry, validator, plugin extension API.
- `envelopestest/` — contract test helper for downstream consumers.
- `docs/` — manifest spec, extension API guide.
- `cmd/envelopes-export/`, `codegen/` — module-resolved catalog and TypeScript generation.
- `examples/` — runnable demonstrations of the public API.

## Docs

- [`docs/manifest-spec.md`](docs/manifest-spec.md) — manifest format.
- [`docs/extension-api.md`](docs/extension-api.md) — plugin extension API.
- [`docs/generation.md`](docs/generation.md) — catalog/codegen API and migration guide.

## Related libraries

`go-envelopes` is part of the Hollis Labs `go-*` portfolio. This module owns
both the Go registry/catalog API and the host-neutral TypeScript generator.
Go and TypeScript hosts therefore consume the same embedded YAML manifest and
JSON Schemas from the module version selected by the host's `go.mod`; there is
no separate TypeScript companion package to install or synchronize.

## Contributing

Issues and PRs welcome. Please run `make test` (which runs
`go test -race -count=1 ./...`) before opening a PR.

## License

MIT — see [LICENSE](LICENSE).
