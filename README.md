# go-envelopes

Shared Go primitive library for the Envelope UI Protocol — manifest, registry, validator, and plugin extension API.

Companion package: `ts-envelopes` (TypeScript). Both publish from the single YAML manifest in `manifest/`.

## Status

`v0.1.0` — initial extraction from Nanite. Reference hosts: Nanite (chat runtime), Tangent (agent-summoned app surface).

## Install

```sh
go get github.com/hollis-labs/go-envelopes
```

## Quick example

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
		V:    1,
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

## Layout

- `manifest/` — canonical YAML manifest + per-type JSON Schemas (single source of truth, language-agnostic).
- `*.go` (root package `envelopes`) — registry, validator, plugin extension API.
- `envelopestest/` — contract test helper for downstream consumers.
- `docs/` — manifest spec, extension API guide, Nanite migration recipe.

## Docs

- [`docs/manifest-spec.md`](docs/manifest-spec.md)
- [`docs/extension-api.md`](docs/extension-api.md)
- [`docs/migration-from-nanite.md`](docs/migration-from-nanite.md)

## License

MIT — see [LICENSE](LICENSE).
