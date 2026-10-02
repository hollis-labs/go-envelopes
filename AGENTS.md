# go-envelopes

The shared primitive for the Envelope UI Protocol — a wire format for typed,
host-rendered payloads that agents send to host applications. It owns the
manifest-driven type registry, the JSON-Schema validator, the runtime plugin
extension API, and the build-time catalog and TypeScript generator. The root
envelope packages remain transport-agnostic (no MCP, no SSE) and store nothing.
The module also owns the independent admin contract in
`admin`; its optional `admin/adminhttp` binding is the only package permitted
to import `net/http`. Storage, authn/z, resolution and lifecycle remain host
concerns. `admin/adminhttp` binds caller-specific declarations and guarded
commands to a host-mounted handler; it starts no server or lifecycle action.

## Start Here

- `README.md` covers the quickstart and the build-time generation commands.
- `admin/README.md` covers admin declarations, private/public values, validation
  and the atomic host adapter contract.
- `admin/adminhttp/README.md` covers required host policies, HTTP commands,
  preconditions and the stage-before-commit boundary.
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

`make lint` and `make vuln` skip with a message when `staticcheck` or
`govulncheck` is absent, so a clean run does not prove they executed.

## Boundaries

The wire-format version (`Envelope.V`) is independent of the library version.
Do not bump one because the other moved. `admin.ContractVersion` is likewise
independent of both module version and `Envelope.V`. Root envelope packages
must not import `admin`; only `admin/adminhttp` may import `net/http`. Admin
declarations are unrelated to the envelope type catalog: do not register them
in `manifest/envelopes.yaml` or change the catalog for admin work.

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
