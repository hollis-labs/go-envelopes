package envelopes

import (
	"context"
	"encoding/json"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func TestExportCatalog_isDeterministicAndSelfIdentifying(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("two unchanged catalog exports differ")
	}
	if first.Source.Module != ModulePath || first.Source.ModuleVersion == "" {
		t.Fatalf("source identity = %#v", first.Source)
	}
	if first.Source.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocol identity = %d, want %d", first.Source.ProtocolVersion, ProtocolVersion)
	}
	if !strings.HasPrefix(first.Source.ManifestDigest, "sha256:") || !strings.HasPrefix(first.CatalogDigest, "sha256:") {
		t.Fatalf("missing digests: source=%q catalog=%q", first.Source.ManifestDigest, first.CatalogDigest)
	}
	if first.ManifestYAML == "" || len(first.ManifestSchema) == 0 {
		t.Fatal("catalog omitted canonical manifest assets")
	}
	if len(first.Types) == 0 || len(first.Schemas) == 0 {
		t.Fatalf("catalog empty: %d types, %d schemas", len(first.Types), len(first.Schemas))
	}
	for i := 1; i < len(first.Types); i++ {
		if first.Types[i-1].Name > first.Types[i].Name {
			t.Fatalf("types not sorted: %q before %q", first.Types[i-1].Name, first.Types[i].Name)
		}
	}

	var infoCard *CatalogType
	for index := range first.Types {
		if first.Types[index].Name == "info-card" {
			infoCard = &first.Types[index]
			break
		}
	}
	if infoCard == nil {
		t.Fatal("catalog omitted info-card")
	}
	// DataType is derived from the wire type name and stays asserted. The
	// companion assertion here used to be Import.Export == "InfoCard", which
	// pinned a host component symbol into the catalog contract; CW-20260910-0113
	// removed the field, so the assertion inverts rather than disappears.
	if infoCard.TypeScript.DataType != "InfoCardData" {
		t.Fatalf("info-card TypeScript metadata = %#v", infoCard.TypeScript)
	}
	if infoCard.TypeScript.Import.Component != "" || infoCard.TypeScript.Import.Export != "" {
		t.Fatalf("core catalog carries a host component binding: %#v", infoCard.TypeScript.Import)
	}
	if infoCard.Annotations.Custom["default_render_target"] != "bottom_chat_drawer" {
		t.Fatalf("info-card annotations = %#v", infoCard.Annotations)
	}
}

func TestModuleIdentityFromBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		want moduleIdentity
	}{
		{
			name: "ordinary selected dependency",
			info: &debug.BuildInfo{Main: debug.Module{Path: "consumer.test"}, Deps: []*debug.Module{{
				Path: ModulePath, Version: "v1.2.3",
			}}},
			want: moduleIdentity{Path: ModulePath, Version: "v1.2.3"},
		},
		{
			name: "local replacement main command",
			info: &debug.BuildInfo{Main: debug.Module{
				Path: ModulePath, Version: "v1.2.3", Replace: &debug.Module{Path: "/private/source", Version: "(devel)"},
			}},
			want: moduleIdentity{Path: ModulePath, Version: "(devel; local replacement)"},
		},
		{
			name: "local replacement",
			info: &debug.BuildInfo{Main: debug.Module{Path: "consumer.test"}, Deps: []*debug.Module{{
				Path: ModulePath, Version: "v1.2.3", Replace: &debug.Module{Path: "/private/source", Version: "(devel)"},
			}}},
			want: moduleIdentity{Path: ModulePath, Version: "(devel; local replacement)"},
		},
		{
			name: "versioned replacement",
			info: &debug.BuildInfo{Main: debug.Module{Path: "consumer.test"}, Deps: []*debug.Module{{
				Path: ModulePath, Version: "v1.2.3", Replace: &debug.Module{Path: "example.com/envelopes-fork", Version: "v1.4.0"},
			}}},
			want: moduleIdentity{Path: "example.com/envelopes-fork", Version: "v1.4.0"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := moduleIdentityFromBuildInfo(test.info); got != test.want {
				t.Fatalf("module identity = %#v, want %#v", got, test.want)
			}
			if strings.Contains(moduleIdentityFromBuildInfo(test.info).Path, "/private/source") {
				t.Fatal("module identity leaked local replacement path")
			}
		})
	}
}

// TestExportCatalog_includesUnregisteredCompatibilitySchemas exercises the
// compatibility-schema path with a synthetic manifest FS.
//
// It used to assert against `kb-result`, one of five app-specific schemas that
// shipped in manifest/schemas/ without a YAML entry. CW-20260910-0114 deleted
// all five, and the module now ships no unregistered schema at all — every
// schema file maps to a declared core type. Pinning this behaviour to a
// shipped stray meant the test quietly depended on the boundary leak it was
// unrelated to, and would have blocked its removal. The mechanism is still
// supported and still tested; it just no longer needs a stray to demonstrate it.
func TestExportCatalog_includesUnregisteredCompatibilitySchemas(t *testing.T) {
	manifestFS := fstest.MapFS{
		"manifest/envelopes.yaml": &fstest.MapFile{
			Data: []byte("core:\n  - type: fixture-card\n"),
		},
		"manifest/schemas/fixture-card.schema.json": &fstest.MapFile{
			Data: []byte(`{"$id":"fixture-card.schema.json","title":"Fixture Card","type":"object"}`),
		},
		// Shipped, but absent from the YAML manifest above.
		"manifest/schemas/fixture-compat.schema.json": &fstest.MapFile{
			Data: []byte(`{"$id":"fixture-compat.schema.json","title":"Fixture Compat","type":"object"}`),
		},
	}
	registry, err := LoadCore(context.Background(), WithManifestFS(manifestFS))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sawCompat, sawRegistered bool
	for _, resource := range catalog.Schemas {
		switch resource.Type {
		case "fixture-compat":
			if resource.Registered {
				t.Fatal("fixture-compat compatibility schema unexpectedly registered")
			}
			if len(resource.Document) == 0 {
				t.Fatal("fixture-compat compatibility schema document empty")
			}
			sawCompat = true
		case "fixture-card":
			if !resource.Registered {
				t.Fatal("fixture-card is declared in the manifest but exported as unregistered")
			}
			sawRegistered = true
		}
	}
	if !sawCompat {
		t.Fatal("fixture-compat compatibility schema not exported")
	}
	if !sawRegistered {
		t.Fatal("fixture-card registered schema not exported")
	}
}

// TestExportCatalog_shipsNoUnregisteredSchemas is the CW-20260910-0114
// regression gate. Five app-specific schemas (giphy-modal, kb-result,
// resolution-capture, ticket-confirmation, ticket-form) shipped in the shared
// manifest/schemas/ directory with no entry in envelopes.yaml. Every schema
// this module ships must now correspond to a declared core type: a schema with
// no type is either app-specific leakage or a type someone forgot to declare,
// and both are defects in a library other applications treat as a contract.
func TestExportCatalog_shipsNoUnregisteredSchemas(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range catalog.Schemas {
		if !resource.Registered {
			t.Errorf("module ships schema %q with no declared type in manifest/envelopes.yaml; "+
				"declare the type or remove the schema", resource.Type)
		}
	}
}

func TestExportCatalog_pluginRegistrationUsesSameSurface(t *testing.T) {
	registry := NewRegistry()
	manifest := []byte(`type: demo.notice
version: 1.2.3
description: Plugin notice
ui:
  component: plugin/Notice
  export: Notice
`)
	schema := []byte(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`)
	if err := registry.RegisterTypeFromManifest(manifest, schema, "demo"); err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Types) != 1 || len(catalog.Schemas) != 1 {
		t.Fatalf("plugin catalog = %d types, %d schemas", len(catalog.Types), len(catalog.Schemas))
	}
	entry := catalog.Types[0]
	if entry.Name != "demo.notice" || entry.Source != "plugin" || entry.PluginID != "demo" {
		t.Fatalf("plugin catalog entry = %#v", entry)
	}
	if entry.SchemaURI != catalog.Schemas[0].URI || !catalog.Schemas[0].Registered {
		t.Fatalf("plugin schema linkage = entry %q resource %#v", entry.SchemaURI, catalog.Schemas[0])
	}
	if removed := registry.UnregisterPlugin("demo"); removed != 1 {
		t.Fatalf("UnregisterPlugin removed %d types, want 1", removed)
	}
	after, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Types) != 0 || len(after.Schemas) != 0 {
		t.Fatalf("plugin catalog retained unloaded entries: %d types, %d schemas", len(after.Types), len(after.Schemas))
	}
}

func TestRegistryLookup_doesNotExposeMutableMetadata(t *testing.T) {
	typedSlice := []string{"original"}
	typedMap := map[string]string{"key": "original"}
	type nestedMetadata struct {
		Label  string            `json:"label"`
		Values map[string]string `json:"values"`
	}
	pointer := &nestedMetadata{Label: "original", Values: map[string]string{"key": "original"}}
	registry := NewRegistry()
	if err := registry.RegisterType(TypeSpec{
		Name:     "demo.metadata",
		PluginID: "demo",
		UIMetadata: map[string]any{
			"typedSlice": typedSlice,
			"typedMap":   typedMap,
			"pointer":    pointer,
		},
		TypeScript: TypeScriptMetadata{Import: ImportMetadata{Extra: map[string]any{
			"typedSlice": typedSlice,
			"typedMap":   typedMap,
			"pointer":    pointer,
		}}},
	}); err != nil {
		t.Fatal(err)
	}

	// Mutating every caller-owned composite after registration must not change
	// registry storage or race with readers.
	typedSlice[0] = "input-mutated"
	typedMap["key"] = "input-mutated"
	pointer.Label = "input-mutated"
	pointer.Values["key"] = "input-mutated"

	assertOriginal := func(t *testing.T, values map[string]any) {
		t.Helper()
		if got := values["typedSlice"].([]any)[0]; got != "original" {
			t.Fatalf("typed slice = %v", got)
		}
		if got := values["typedMap"].(map[string]any)["key"]; got != "original" {
			t.Fatalf("typed map = %v", got)
		}
		nested := values["pointer"].(map[string]any)
		if nested["label"] != "original" || nested["values"].(map[string]any)["key"] != "original" {
			t.Fatalf("pointer value = %#v", nested)
		}
	}

	lookup, ok := registry.Lookup("demo.metadata")
	if !ok {
		t.Fatal("demo.metadata missing")
	}
	assertOriginal(t, lookup.UIMetadata)
	assertOriginal(t, lookup.TypeScript.Import.Extra)
	lookup.UIMetadata["typedSlice"].([]any)[0] = "lookup-mutated"
	lookup.TypeScript.Import.Extra["typedMap"].(map[string]any)["key"] = "lookup-mutated"
	all := registry.All()
	assertOriginal(t, all[0].UIMetadata)
	assertOriginal(t, all[0].TypeScript.Import.Extra)

	firstCatalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	assertOriginal(t, firstCatalog.Types[0].TypeScript.Import.Extra)
	firstCatalog.Types[0].TypeScript.Import.Extra["pointer"].(map[string]any)["label"] = "catalog-mutated"
	secondCatalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	assertOriginal(t, secondCatalog.Types[0].TypeScript.Import.Extra)

	// Each read owns its return value. Concurrent mutation of those snapshots is
	// therefore race-free with other reads and exports.
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			copy, _ := registry.Lookup("demo.metadata")
			copy.UIMetadata["typedSlice"].([]any)[0] = "concurrent"
			copy.TypeScript.Import.Extra["typedMap"].(map[string]any)["key"] = "concurrent"
			all := registry.All()
			all[0].UIMetadata["pointer"].(map[string]any)["label"] = "concurrent"
			catalog, exportErr := registry.ExportCatalog()
			if exportErr == nil {
				catalog.Types[0].TypeScript.Import.Extra["typedSlice"].([]any)[0] = "concurrent"
			}
		}()
	}
	wait.Wait()
	again, _ := registry.Lookup("demo.metadata")
	assertOriginal(t, again.UIMetadata)
}

func TestRegisterType_rejectsNonJSONExtensionMetadata(t *testing.T) {
	registry := NewRegistry()
	err := registry.RegisterType(TypeSpec{
		Name:     "demo.bad-metadata",
		PluginID: "demo",
		UIMetadata: map[string]any{
			"custom": func() {},
		},
	})
	if err == nil {
		t.Fatal("RegisterType accepted non-JSON extension metadata")
	}
	if registry.Has("demo.bad-metadata") {
		t.Fatal("failed metadata normalization inserted a partial registration")
	}
}
