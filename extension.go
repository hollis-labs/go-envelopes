package envelopes

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// pluginTypeName matches a namespaced plugin type name: two or more
// kebab-case-lower segments joined by single dots (e.g.
// "myplugin.calendar-pick", "myorg.myplugin.calendar"). Plugin types
// MUST be namespaced; un-namespaced names (no dot) are reserved for
// core types.
//
// Multi-segment names are accepted so plugin authors can stack a
// vendor/org prefix in front of the plugin id ("myorg.myplugin.kind")
// without the registry imposing a structural opinion. The library
// treats the whole string as the type name; the conventional "first
// segment is the plugin id" reading is up to the caller.
var pluginTypeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*(?:\.[a-z0-9][a-z0-9-]*)+$`)

// RegisterType adds a plugin-owned envelope type to the registry. Used by
// plugin-SDK consumers to extend the catalog at runtime.
//
// Rules:
//   - spec.Name MUST be a namespaced kebab-case identifier with at least
//     one dot separator: "<plugin-id>.<type>" minimally, optionally with
//     a vendor/org prefix like "<vendor>.<plugin-id>.<type>". Each
//     segment matches [a-z0-9][a-z0-9-]*. Un-namespaced names (no dot)
//     are reserved for core types and rejected with ErrInvalidName.
//   - spec.Source is forced to TypeSourcePlugin regardless of input,
//     keeping UnregisterType's core-type protection intact. PluginID is
//     preserved as supplied.
//   - The first registration wins. A second call for the same name
//     returns ErrConflict; the library does not silently overwrite.
//
// Trust evaluation (signed plugins, capability gating, etc.) is the
// surrounding plugin-SDK's responsibility — this function accepts any
// well-formed registration. See docs/extension-api.md for the boundary.
func (r *Registry) RegisterType(spec TypeSpec) error {
	if !pluginTypeName.MatchString(spec.Name) {
		return fmt.Errorf("%w: %q must be a namespaced kebab-case name with at least one dot (e.g. <plugin-id>.<type>)", ErrInvalidName, spec.Name)
	}
	// Plugin-side registrations always land as TypeSourcePlugin. Callers
	// that pass a different value get it overwritten, which keeps
	// UnregisterType's core-type protection from being corrupted by a
	// misbehaving caller.
	spec.Source = TypeSourcePlugin
	if spec.ResponseKind == "" {
		spec.ResponseKind = ResponseKindData
	}
	if !spec.ResponseKind.IsValid() {
		return fmt.Errorf("%w: %q", ErrUnsupportedKind, spec.ResponseKind)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.types[spec.Name]; exists {
		return fmt.Errorf("%w: %q", ErrConflict, spec.Name)
	}
	r.types[spec.Name] = spec
	return nil
}

// UnregisterType removes a plugin-owned type from the registry. Returns
// ErrUnknownType if the name is not registered, or ErrCoreTypeProtected
// if the caller attempts to remove a core type.
//
// Used when a plugin is unloaded so stale entries don't survive a hot
// unload/reload cycle.
func (r *Registry) UnregisterType(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	spec, ok := r.types[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownType, name)
	}
	if spec.Source == TypeSourceCore {
		return fmt.Errorf("%w: %q", ErrCoreTypeProtected, name)
	}
	delete(r.types, name)
	return nil
}

// UnregisterPlugin removes every type whose PluginID matches the supplied
// id. Returns the number of types removed. Convenient when a plugin host
// loads multiple types from one plugin and wants to drop them atomically
// during unload.
func (r *Registry) UnregisterPlugin(pluginID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for name, spec := range r.types {
		if spec.Source == TypeSourceCore {
			continue
		}
		if spec.PluginID != pluginID {
			continue
		}
		delete(r.types, name)
		n++
	}
	return n
}

// PluginManifestEntry is the YAML/JSON shape a plugin uses to declare one
// or more envelope types. Plugins typically ship a manifest file alongside
// their schema files; the host loads them via RegisterTypeFromManifest.
type PluginManifestEntry struct {
	Type         string `yaml:"type" json:"type"`
	Version      string `yaml:"version,omitempty" json:"version,omitempty"`
	Description  string `yaml:"description,omitempty" json:"description,omitempty"`
	ResponseKind string `yaml:"responseKind,omitempty" json:"responseKind,omitempty"`
	// UIMetadata is a passthrough bag for TS-side rendering hints
	// (component, export, props, etc.). The Go side does not interpret it.
	UIMetadata map[string]any `yaml:"ui,omitempty" json:"ui,omitempty"`
}

// RegisterTypeFromManifest is a convenience for plugins that ship a
// manifest fragment plus an optional JSON Schema file. manifestBytes may
// be YAML or JSON — both decode into PluginManifestEntry. schemaBytes may
// be empty when the type has no per-instance data shape.
//
// pluginID is recorded on the resulting TypeSpec.PluginID for later
// UnregisterPlugin calls. The final registered name is the manifest
// entry's Type verbatim when it already satisfies the namespaced form
// (one or more dot-separated kebab segments — e.g. "myplugin.calendar"
// or "myorg.myplugin.calendar"); otherwise it is constructed by
// prefixing pluginID, yielding "<pluginID>.<type>".
func (r *Registry) RegisterTypeFromManifest(manifestBytes, schemaBytes []byte, pluginID string) error {
	if pluginID == "" {
		return fmt.Errorf("envelopes: plugin id is required")
	}
	var entry PluginManifestEntry
	if err := decodeManifestEntry(manifestBytes, &entry); err != nil {
		return err
	}
	if entry.Type == "" {
		return fmt.Errorf("envelopes: manifest entry missing type")
	}
	name := entry.Type
	if !pluginTypeName.MatchString(name) {
		// Caller supplied an unnamespaced segment; prefix with pluginID.
		// The combined form must satisfy the namespaced regex.
		name = pluginID + "." + entry.Type
		if !pluginTypeName.MatchString(name) {
			return fmt.Errorf("%w: %q (combined with plugin id %q)", ErrInvalidName, name, pluginID)
		}
	}

	spec := TypeSpec{
		Name:         name,
		Version:      entry.Version,
		ResponseKind: ResponseKindData,
		Description:  entry.Description,
		Source:       TypeSourcePlugin,
		PluginID:     pluginID,
		UIMetadata:   entry.UIMetadata,
	}
	if entry.ResponseKind != "" {
		spec.ResponseKind = ResponseKind(entry.ResponseKind)
	}

	if len(schemaBytes) > 0 {
		schema, err := compilePluginSchema(pluginID, name, schemaBytes)
		if err != nil {
			return err
		}
		spec.DataSchema = schema
	}
	return r.RegisterType(spec)
}

// decodeManifestEntry tries YAML first (the canonical manifest format) and
// falls back to JSON. Either lands the same struct shape.
func decodeManifestEntry(data []byte, out *PluginManifestEntry) error {
	if err := yaml.Unmarshal(data, out); err == nil && out.Type != "" {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("envelopes: parse plugin manifest: %w", err)
	}
	return nil
}

// compilePluginSchema compiles a plugin-supplied JSON Schema with a
// stable in-memory URI so error messages don't leak filesystem layout.
func compilePluginSchema(pluginID, typeName string, schemaBytes []byte) (*jsonschema.Schema, error) {
	var doc any
	if err := json.Unmarshal(schemaBytes, &doc); err != nil {
		return nil, fmt.Errorf("envelopes: parse schema for %q: %w", typeName, err)
	}
	uri := fmt.Sprintf("plugin://%s/envelopes/%s.schema.json", pluginID, typeName)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(uri, doc); err != nil {
		return nil, fmt.Errorf("envelopes: register schema for %q: %w", typeName, err)
	}
	compiled, err := c.Compile(uri)
	if err != nil {
		return nil, fmt.Errorf("envelopes: compile schema for %q: %w", typeName, err)
	}
	return compiled, nil
}
