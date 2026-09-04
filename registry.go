package envelopes

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

// Registry holds the catalog of envelope types known to one process —
// core types loaded from the embedded manifest plus any plugin types
// added at runtime via the extension API.
//
// A Registry is safe for concurrent use. Reads (Lookup, All, Validate*)
// take an RLock; Register/Unregister take a write lock. The registry
// holds compiled *jsonschema.Schema values; compilation cost is paid at
// load/registration time so validation is cheap.
type Registry struct {
	mu              sync.RWMutex
	types           map[string]TypeSpec
	manifestYAML    []byte
	manifestSchema  []byte
	schemaResources map[string]SchemaResource
}

// NewRegistry returns an empty registry. Most consumers want LoadCore,
// which constructs an empty registry and seeds it with core types.
func NewRegistry() *Registry {
	return &Registry{
		types:           make(map[string]TypeSpec),
		schemaResources: make(map[string]SchemaResource),
	}
}

// LoadOption configures LoadCore.
type LoadOption func(*loadConfig)

type loadConfig struct {
	manifestFS fs.FS
}

// WithManifestFS overrides the embedded manifest filesystem with a
// caller-supplied one. The supplied FS must be rooted such that
// "manifest/envelopes.yaml" and "manifest/schemas/<type>.schema.json"
// resolve.
//
// Use this for tests, snapshot fixtures, or out-of-band manifest
// distribution. By default LoadCore reads the FS produced by EmbeddedFS().
func WithManifestFS(f fs.FS) LoadOption {
	return func(c *loadConfig) { c.manifestFS = f }
}

// LoadCore parses the embedded YAML manifest, compiles each per-type
// schema present on disk, and registers all entries with TypeSourceCore.
//
// Entries without an accompanying schema file (e.g. message-* in the
// core seed) register with a nil DataSchema; ValidateEnvelope skips
// schema validation for those types and only confirms the type is
// known. This preserves the seed's "declared without schema" behavior
// for types whose payload shape is intentionally open.
func LoadCore(ctx context.Context, opts ...LoadOption) (*Registry, error) {
	cfg := loadConfig{manifestFS: embeddedManifest}
	for _, o := range opts {
		o(&cfg)
	}

	manifestBytes, err := fs.ReadFile(cfg.manifestFS, "manifest/envelopes.yaml")
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, err
	}

	r := NewRegistry()
	r.manifestYAML = append([]byte(nil), manifestBytes...)
	if schema, readErr := fs.ReadFile(cfg.manifestFS, "manifest/envelopes.schema.json"); readErr == nil {
		r.manifestSchema = append([]byte(nil), schema...)
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("read manifest schema: %w", readErr)
	}
	if err := r.loadSchemaResources(cfg.manifestFS); err != nil {
		return nil, err
	}
	for _, entry := range manifest.Core {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		spec := TypeSpec{
			Name:         entry.Type,
			Version:      "1.0.0",
			ResponseKind: ResponseKindData,
			Description:  entry.Description,
			Source:       TypeSourceCore,
			TypeScript: TypeScriptMetadata{
				DataType: TypeScriptDataTypeName(entry.Type),
				Import:   importMetadataForEntry(entry),
			},
			UIMetadata: uiMetadataForEntry(entry),
		}
		schemaPath := schemaPathForType(entry.Type)
		compiled, document, err := compileSchemaFromFS(cfg.manifestFS, schemaPath, "embedded://"+schemaPath)
		switch {
		case err == nil:
			spec.DataSchema = compiled
			spec.DataSchemaDocument = document
		case errors.Is(err, fs.ErrNotExist):
			// No per-type schema shipped — register without one.
		default:
			return nil, fmt.Errorf("core type %q: %w", entry.Type, err)
		}
		if err := r.registerLocked(spec); err != nil {
			return nil, fmt.Errorf("register core type %q: %w", entry.Type, err)
		}
	}
	return r, nil
}

func (r *Registry) loadSchemaResources(f fs.FS) error {
	entries, err := fs.ReadDir(f, "manifest/schemas")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read schema directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}
		typeName := strings.TrimSuffix(entry.Name(), ".schema.json")
		schemaPath := path.Join("manifest", "schemas", entry.Name())
		raw, err := fs.ReadFile(f, schemaPath)
		if err != nil {
			return fmt.Errorf("read schema %s: %w", schemaPath, err)
		}
		document, err := NewSchemaDocument("embedded://"+schemaPath, raw)
		if err != nil {
			return fmt.Errorf("load schema %s: %w", schemaPath, err)
		}
		r.schemaResources[typeName] = SchemaResource{
			Type:     typeName,
			URI:      document.URI(),
			Source:   TypeSourceCore.String(),
			Document: document.JSON(),
		}
	}
	return nil
}

// Lookup returns the TypeSpec for the named type. The boolean is false if
// the name is not registered. The returned spec is a copy; callers may not
// mutate the registry through it.
func (r *Registry) Lookup(name string) (TypeSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.types[name]
	return cloneTypeSpec(spec), ok
}

// Has reports whether the named type is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	_, ok := r.types[name]
	r.mu.RUnlock()
	return ok
}

// All returns every registered type spec, sorted by Name. The slice is
// freshly allocated; the caller may sort it differently or filter without
// affecting the registry.
func (r *Registry) All() []TypeSpec {
	r.mu.RLock()
	out := make([]TypeSpec, 0, len(r.types))
	for _, spec := range r.types {
		out = append(out, cloneTypeSpec(spec))
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns all registered type names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	out := make([]string, 0, len(r.types))
	for name := range r.types {
		out = append(out, name)
	}
	r.mu.RUnlock()
	sort.Strings(out)
	return out
}

// Len returns the number of registered types.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.types)
}

// registerLocked inserts a spec into the map. Caller must hold r.mu (write).
// Conflict and naming rules are enforced by RegisterType (the public entry
// point); LoadCore calls this directly because core type names skip the
// plugin namespace check.
func (r *Registry) registerLocked(spec TypeSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if _, exists := r.types[spec.Name]; exists {
		return fmt.Errorf("%w: %q", ErrConflict, spec.Name)
	}
	if spec.TypeScript.DataType == "" {
		spec.TypeScript.DataType = TypeScriptDataTypeName(spec.Name)
	}
	var err error
	spec.UIMetadata, err = normalizeStringAnyMap(spec.UIMetadata)
	if err != nil {
		return fmt.Errorf("envelopes: normalize UI metadata for %q: %w", spec.Name, err)
	}
	spec.TypeScript.Import.Extra, err = normalizeStringAnyMap(spec.TypeScript.Import.Extra)
	if err != nil {
		return fmt.Errorf("envelopes: normalize TypeScript import metadata for %q: %w", spec.Name, err)
	}
	r.types[spec.Name] = spec
	r.recordSchemaResourceLocked(spec)
	return nil
}

func (r *Registry) recordSchemaResourceLocked(spec TypeSpec) {
	if spec.DataSchemaDocument == nil {
		return
	}
	r.schemaResources[spec.Name] = SchemaResource{
		Type:     spec.Name,
		URI:      spec.DataSchemaDocument.URI(),
		Source:   spec.Source.String(),
		PluginID: spec.PluginID,
		Document: spec.DataSchemaDocument.JSON(),
	}
}

func cloneTypeSpec(spec TypeSpec) TypeSpec {
	spec.UIMetadata = cloneStringAnyMap(spec.UIMetadata)
	spec.TypeScript = cloneTypeScriptMetadata(spec.TypeScript)
	return spec
}

// defaultRegistry is the lazy singleton returned by Default().
var (
	defaultOnce sync.Once
	defaultReg  *Registry
	defaultErr  error
)

// Default returns the package-level registry, loading it on first call.
// Convenient for apps that want a single shared catalog without managing
// lifetime; apps that need isolation should construct their own via
// LoadCore.
//
// The first call may return an error if the embedded manifest is malformed.
// Subsequent calls return the same registry (and the same error) without
// re-loading.
func Default() (*Registry, error) {
	defaultOnce.Do(func() {
		defaultReg, defaultErr = LoadCore(context.Background())
	})
	return defaultReg, defaultErr
}
