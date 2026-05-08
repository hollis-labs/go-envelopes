package envelopes

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// embeddedManifest carries the canonical YAML manifest plus per-type JSON
// Schemas. Consumers that ship the lib in a binary get the catalog without
// any filesystem dependency. The same files are also published at the repo
// root for ts-envelopes and other downstream tools.
//
//go:embed manifest/envelopes.yaml manifest/envelopes.schema.json manifest/schemas/*.schema.json
var embeddedManifest embed.FS

// EmbeddedFS exposes the in-binary manifest filesystem so consumers can
// inspect the raw YAML and JSON Schemas directly (e.g. for documentation
// generation). The returned fs.FS is rooted at the repo root, so paths
// look like "manifest/envelopes.yaml".
func EmbeddedFS() fs.FS { return embeddedManifest }

// ManifestEntry models one record under the YAML manifest's "core" list.
// Field names mirror the canonical YAML; unknown fields are preserved in
// Extra so downstream tooling can read non-Go metadata without changes
// here when the manifest grows.
type ManifestEntry struct {
	Type        string `yaml:"type"`
	Component   string `yaml:"component,omitempty"`
	Export      string `yaml:"export,omitempty"`
	Description string `yaml:"description,omitempty"`
	Props       string `yaml:"props,omitempty"`
}

// Manifest is the parsed top-level YAML manifest.
type Manifest struct {
	Core []ManifestEntry `yaml:"core"`
}

// ParseManifest decodes the YAML manifest body. It does NOT validate
// against the manifest's own JSON Schema; callers that want metaschema
// enforcement should run their own validation step against the embedded
// envelopes.schema.json.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// uiMetadataForEntry returns the entry's TS-side rendering hints as a map
// the registry can store on its TypeSpec. Empty when the entry has no
// rendering metadata.
func uiMetadataForEntry(e ManifestEntry) map[string]any {
	out := map[string]any{}
	if e.Component != "" {
		out["component"] = e.Component
	}
	if e.Export != "" {
		out["export"] = e.Export
	}
	if e.Props != "" {
		out["props"] = e.Props
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// compileSchemaFromFS reads, parses, and compiles a JSON Schema from the
// given filesystem at the given path. Returns nil, fs.ErrNotExist if the
// schema file is absent — the caller distinguishes "no schema" from a real
// parse/compile failure.
func compileSchemaFromFS(f fs.FS, schemaPath, resourceURI string) (*jsonschema.Schema, error) {
	raw, err := fs.ReadFile(f, schemaPath)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse schema %s: %w", schemaPath, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(resourceURI, doc); err != nil {
		return nil, fmt.Errorf("register schema %s: %w", schemaPath, err)
	}
	compiled, err := c.Compile(resourceURI)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", schemaPath, err)
	}
	return compiled, nil
}

// schemaPathForType returns the canonical schema file path for a core
// envelope type within the manifest filesystem. Mirrors Nanite's prior
// "schemas/<type>.schema.json" convention.
func schemaPathForType(typeName string) string {
	return path.Join("manifest", "schemas", typeName+".schema.json")
}
