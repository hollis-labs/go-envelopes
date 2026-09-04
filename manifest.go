package envelopes

import (
	"embed"
	"fmt"
	"io/fs"
	"path"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// embeddedManifest carries the canonical YAML manifest plus per-type JSON
// Schemas. Consumers that ship the lib in a binary get the catalog without
// any filesystem dependency. The same files are also exported through the
// module-owned catalog for downstream generation tools.
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
	Type        string         `yaml:"type"`
	Component   string         `yaml:"component,omitempty"`
	Export      string         `yaml:"export,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Props       string         `yaml:"props,omitempty"`
	Extra       map[string]any `yaml:"-"`
}

// UnmarshalYAML preserves unknown entry keys in Extra so build-time metadata
// can flow through the catalog without a library release for every new hint.
func (e *ManifestEntry) UnmarshalYAML(value *yaml.Node) error {
	type manifestEntry ManifestEntry
	var known manifestEntry
	if err := value.Decode(&known); err != nil {
		return err
	}
	*e = ManifestEntry(known)
	var all map[string]any
	if err := value.Decode(&all); err != nil {
		return err
	}
	for _, key := range []string{"type", "component", "export", "description", "props"} {
		delete(all, key)
	}
	if len(all) > 0 {
		e.Extra = all
	}
	return nil
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
	out := cloneStringAnyMap(e.Extra)
	if out == nil {
		out = map[string]any{}
	}
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

func importMetadataForEntry(e ManifestEntry) ImportMetadata {
	return ImportMetadata{
		Component: e.Component,
		Export:    e.Export,
		Props:     e.Props,
		Extra:     cloneStringAnyMap(e.Extra),
	}
}

// compileSchemaFromFS reads, parses, and compiles a JSON Schema from the
// given filesystem at the given path. Returns nil, fs.ErrNotExist if the
// schema file is absent — the caller distinguishes "no schema" from a real
// parse/compile failure.
func compileSchemaFromFS(f fs.FS, schemaPath, resourceURI string) (*jsonschema.Schema, *SchemaDocument, error) {
	raw, err := fs.ReadFile(f, schemaPath)
	if err != nil {
		return nil, nil, err
	}
	document, err := NewSchemaDocument(resourceURI, raw)
	if err != nil {
		return nil, nil, fmt.Errorf("parse schema %s: %w", schemaPath, err)
	}
	compiled, err := compileSchemaDocument(document)
	if err != nil {
		return nil, nil, fmt.Errorf("compile schema %s: %w", schemaPath, err)
	}
	return compiled, document, nil
}

func compileSchemaDocument(document *SchemaDocument) (*jsonschema.Schema, error) {
	if document == nil {
		return nil, fmt.Errorf("envelopes: schema document is nil")
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(document.URI(), document.document); err != nil {
		return nil, fmt.Errorf("register schema %s: %w", document.URI(), err)
	}
	compiled, err := c.Compile(document.URI())
	if err != nil {
		return nil, err
	}
	return compiled, nil
}

// schemaPathForType returns the canonical schema file path for a core
// envelope type within the manifest filesystem. The convention is
// "manifest/schemas/<type>.schema.json".
func schemaPathForType(typeName string) string {
	return path.Join("manifest", "schemas", typeName+".schema.json")
}
