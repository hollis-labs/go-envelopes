# go-envelopes

The shared primitive for the Envelope UI Protocol — a wire format for typed,
host-rendered payloads that agents send to host applications. It owns the
manifest-driven type registry, the JSON-Schema validator, the runtime plugin
extension API, and the build-time catalog and TypeScript generator. It is
transport-agnostic (no MCP, no SSE) and stores nothing; both are host concerns.

## Start Here

- `README.md` covers the quickstart and the build-time generation commands.
- `manifest/envelopes.yaml` is the source of truth for core types;
  `manifest/envelopes.schema.json` constrains it.
- `docs/manifest-spec.md`, `docs/extension-api.md` and `docs/generation.md`
  are the reference documents for those three surfaces.
- `registry.go` and `catalog.go` own type registration and `LoadCore`.
- `validator.go` owns envelope validation; `types.go` owns the wire types.
- `extension.go` is the plugin registration surface.
- `codegen/typescript.go` and `cmd/envelopes-export/main.go` produce the
  catalog and TypeScript outputs.
- `envelopestest/contract.go` is the contract suite downstream hosts run.

## Commands

```bash
make build
make test
make lint
make vuln
```

`make lint` and `make vuln` report three outcomes and only one of them exits 0:
the tool ran and found nothing. A tool that is absent says `DID NOT RUN` and
fails; findings say so and fail. Read the message — `make` reports its own
exit 2 for both failure cases, so the message is the discriminator, not the
code.

## Boundaries

The wire-format version (`Envelope.V`) is independent of the library version.
Do not bump one because the other moved.

`manifest/envelopes.yaml` is the source of truth; the Go types, catalog and
TypeScript output are all derived from it. Edit the manifest and regenerate —
hand-editing generated output puts the two out of sync silently.

Exported catalogs must be deterministic and self-identifying (module version,
protocol version, manifest digest), because consumers diff them across
upgrades — `TestExportCatalog_isDeterministicAndSelfIdentifying`.

Compatibility values are retained deliberately, not left behind. The legacy
`cancelled` response status stays accepted through the v0.4.x window while not
being canonical, and `UIMetadata` remains for v0.3 source compatibility.
`TestCancellationVocabularyV04CompatibilityWindow` and
`TestCancellationVocabularyReadsV02ResponseJSON` are what stop a cleanup from
breaking existing hosts.

`envelopestest.RunContract` is a public API that downstream hosts call from
their own suites. Weakening an assertion there weakens everyone's regression
gate at once.
