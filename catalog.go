package envelopes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime/debug"
	"sort"
	"strings"
	"unicode"
)

const (
	// ModulePath is the canonical Go module that owns this catalog.
	ModulePath = "github.com/hollis-labs/go-envelopes"

	// CatalogFormatVersion versions the JSON shape returned by ExportCatalog.
	CatalogFormatVersion = 1
)

// SourceIdentity identifies the module and exact embedded content that
// produced a catalog. ModuleVersion is the version selected by the consuming
// Go build; it is "(devel)" only for an unversioned local build.
type SourceIdentity struct {
	Module          string `json:"module"`
	ModuleVersion   string `json:"moduleVersion"`
	ProtocolVersion int    `json:"protocolVersion"`
	ManifestDigest  string `json:"manifestDigest"`
}

// Catalog is a deterministic, serializable view of a Registry. It is the
// stable build-time handoff for non-Go generators: the canonical manifest,
// its schema, every schema resource, typed import metadata, and plugin types
// all travel through the same object.
type Catalog struct {
	FormatVersion  int              `json:"formatVersion"`
	Source         SourceIdentity   `json:"source"`
	CatalogDigest  string           `json:"catalogDigest"`
	ManifestYAML   string           `json:"manifestYAML,omitempty"`
	ManifestSchema json.RawMessage  `json:"manifestSchema,omitempty"`
	Types          []CatalogType    `json:"types"`
	Schemas        []SchemaResource `json:"schemas"`
}

// CatalogType is the generator-facing view of a registered envelope type.
type CatalogType struct {
	Name         string             `json:"name"`
	Version      string             `json:"version,omitempty"`
	Description  string             `json:"description,omitempty"`
	ResponseKind ResponseKind       `json:"responseKind"`
	Source       string             `json:"source"`
	PluginID     string             `json:"pluginId,omitempty"`
	TypeScript   TypeScriptMetadata `json:"typescript"`
	SchemaURI    string             `json:"schemaURI,omitempty"`
	Annotations  *SchemaMetadata    `json:"annotations,omitempty"`
}

// SchemaResource is a raw schema shipped by the module or registered by a
// plugin. Registered reports whether the resource is attached to a CatalogType;
// core distributions may retain compatibility schemas that are intentionally
// not in the live registry.
type SchemaResource struct {
	Type       string          `json:"type"`
	URI        string          `json:"uri"`
	Source     string          `json:"source"`
	PluginID   string          `json:"pluginId,omitempty"`
	Registered bool            `json:"registered"`
	Document   json.RawMessage `json:"document"`
}

// ExportCatalog returns a deep, deterministic snapshot of the registry. It
// includes plugin registrations and their schemas when they were registered
// with a SchemaDocument (RegisterTypeFromManifest does this automatically).
// It returns an error when caller-supplied extension metadata cannot be
// represented as JSON.
func (r *Registry) ExportCatalog() (Catalog, error) {
	r.mu.RLock()
	manifestYAML := append([]byte(nil), r.manifestYAML...)
	manifestSchema := append(json.RawMessage(nil), r.manifestSchema...)
	types := make([]CatalogType, 0, len(r.types))
	registered := make(map[string]bool, len(r.types))
	for name, spec := range r.types {
		registered[name] = true
		catalogType := CatalogType{
			Name:         spec.Name,
			Version:      spec.Version,
			Description:  spec.Description,
			ResponseKind: spec.ResponseKind,
			Source:       spec.Source.String(),
			PluginID:     spec.PluginID,
			TypeScript:   cloneTypeScriptMetadata(spec.TypeScript),
		}
		if spec.DataSchemaDocument != nil {
			catalogType.SchemaURI = spec.DataSchemaDocument.URI()
			metadata := spec.DataSchemaDocument.Metadata()
			catalogType.Annotations = &metadata
		}
		types = append(types, catalogType)
	}
	schemas := make([]SchemaResource, 0, len(r.schemaResources))
	for _, resource := range r.schemaResources {
		resource.Document = append(json.RawMessage(nil), resource.Document...)
		resource.Registered = registered[resource.Type]
		schemas = append(schemas, resource)
	}
	r.mu.RUnlock()

	sort.Slice(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	sort.Slice(schemas, func(i, j int) bool {
		if schemas[i].Type == schemas[j].Type {
			return schemas[i].URI < schemas[j].URI
		}
		return schemas[i].Type < schemas[j].Type
	})
	coreResources := coreSchemas(schemas)
	manifestDigest := ""
	if len(manifestYAML) > 0 || len(manifestSchema) > 0 || len(coreResources) > 0 {
		manifestDigest = digestCatalogParts(string(manifestYAML), manifestSchema, coreResources)
	}
	catalog := Catalog{
		FormatVersion: CatalogFormatVersion,
		Source: SourceIdentity{
			Module:          ModulePath,
			ModuleVersion:   selectedModuleVersion(),
			ProtocolVersion: ProtocolVersion,
			ManifestDigest:  manifestDigest,
		},
		ManifestYAML:   string(manifestYAML),
		ManifestSchema: manifestSchema,
		Types:          types,
		Schemas:        schemas,
	}
	digest, err := digestCatalog(catalog)
	if err != nil {
		return Catalog{}, err
	}
	catalog.CatalogDigest = digest
	return catalog, nil
}

func selectedModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	if info.Main.Path == ModulePath {
		if info.Main.Version != "" {
			return info.Main.Version
		}
		return "(devel)"
	}
	for _, dep := range info.Deps {
		if dep.Path != ModulePath {
			continue
		}
		if dep.Version != "" {
			return dep.Version
		}
		return "(devel)"
	}
	return "(devel)"
}

func digestCatalog(catalog Catalog) (string, error) {
	catalog.CatalogDigest = ""
	raw, err := json.Marshal(catalog)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func digestCatalogParts(manifest string, manifestSchema json.RawMessage, schemas []SchemaResource) string {
	h := sha256.New()
	_, _ = h.Write([]byte("manifest/envelopes.yaml\x00"))
	_, _ = h.Write([]byte(manifest))
	_, _ = h.Write([]byte("\x00manifest/envelopes.schema.json\x00"))
	_, _ = h.Write(manifestSchema)
	for _, schema := range schemas {
		_, _ = h.Write([]byte("\x00" + schema.Type + "\x00" + schema.URI + "\x00"))
		_, _ = h.Write(schema.Document)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func coreSchemas(resources []SchemaResource) []SchemaResource {
	out := make([]SchemaResource, 0, len(resources))
	for _, resource := range resources {
		if resource.Source == TypeSourceCore.String() {
			out = append(out, resource)
		}
	}
	return out
}

func cloneTypeScriptMetadata(metadata TypeScriptMetadata) TypeScriptMetadata {
	metadata.Import.Extra = cloneStringAnyMap(metadata.Import.Extra)
	return metadata
}

func cloneStringAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = cloneJSONValue(value)
	}
	return out
}

// TypeScriptDataTypeName derives the stable TypeScript data type exported for
// an envelope name (for example, "info-card" becomes "InfoCardData").
func TypeScriptDataTypeName(name string) string {
	var b strings.Builder
	upperNext := true
	first := true
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upperNext = true
			continue
		}
		if upperNext {
			if first && unicode.IsDigit(r) {
				b.WriteString("Envelope")
			}
			b.WriteRune(unicode.ToUpper(r))
			upperNext = false
		} else {
			b.WriteRune(r)
		}
		first = false
	}
	if b.Len() == 0 {
		return "EnvelopeData"
	}
	return b.String() + "Data"
}
