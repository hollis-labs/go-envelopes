package envelopes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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
	if infoCard.TypeScript.DataType != "InfoCardData" || infoCard.TypeScript.Import.Export != "InfoCard" {
		t.Fatalf("info-card TypeScript metadata = %#v", infoCard.TypeScript)
	}
	if infoCard.Annotations.Custom["default_render_target"] != "bottom_chat_drawer" {
		t.Fatalf("info-card annotations = %#v", infoCard.Annotations)
	}
}

func TestExportCatalog_includesUnregisteredCompatibilitySchemas(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range catalog.Schemas {
		if resource.Type == "kb-result" {
			if resource.Registered {
				t.Fatal("kb-result compatibility schema unexpectedly registered")
			}
			if len(resource.Document) == 0 {
				t.Fatal("kb-result compatibility schema document empty")
			}
			return
		}
	}
	t.Fatal("kb-result compatibility schema not exported")
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
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := registry.Lookup("info-card")
	if !ok {
		t.Fatal("info-card missing")
	}
	spec.UIMetadata["component"] = "mutated"
	spec.TypeScript.Import.Component = "also-mutated"
	again, _ := registry.Lookup("info-card")
	if again.UIMetadata["component"] == "mutated" || again.TypeScript.Import.Component == "also-mutated" {
		t.Fatal("Lookup returned mutable registry metadata")
	}
}

func TestExportCatalog_rejectsNonJSONExtensionMetadata(t *testing.T) {
	registry := NewRegistry()
	err := registry.RegisterType(TypeSpec{
		Name:     "demo.bad-metadata",
		PluginID: "demo",
		UIMetadata: map[string]any{
			"custom": func() {},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ExportCatalog(); err == nil {
		t.Fatal("ExportCatalog accepted non-JSON extension metadata")
	}
}
