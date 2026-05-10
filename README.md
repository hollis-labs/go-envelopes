# go-envelopes

`go-envelopes` is the shared Go primitive library for the **Envelope UI
Protocol** — a small wire format for typed, host-rendered payloads that
agents send to host applications. The package ships a manifest-driven
type registry, JSON-Schema validator, and plugin extension API for
runtime registration of additional envelope types.

It is transport-agnostic. It does not bind to MCP, SSE, or any other
transport — those concerns live in the host application. It also does
not persist envelope instances; storage is host-defined.

## Status

`v0.1.x` — pre-1.0. Public API may shift between minor versions; see
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

## Layout

- `manifest/` — canonical YAML manifest + per-type JSON Schemas (single source of truth, language-agnostic).
- `*.go` (root package `envelopes`) — registry, validator, plugin extension API.
- `envelopestest/` — contract test helper for downstream consumers.
- `docs/` — manifest spec, extension API guide.
- `examples/` — runnable demonstrations of the public API.

## Docs

- [`docs/manifest-spec.md`](docs/manifest-spec.md) — manifest format.
- [`docs/extension-api.md`](docs/extension-api.md) — plugin extension API.

## Related libraries

`go-envelopes` is part of the Hollis Labs `go-*` portfolio. It pairs
with a TypeScript companion that consumes the same YAML manifest, so
host applications can render the same envelope catalog whether the host
is written in Go or TypeScript.

## Contributing

Issues and PRs welcome. Please run `make test` (which runs
`go test -race -count=1 ./...`) before opening a PR.

## License

MIT — see [LICENSE](LICENSE).
