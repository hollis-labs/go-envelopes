package envelopes

import (
	"context"
	"testing"
	"testing/fstest"
)

// fixtureManifestFS returns a synthetic manifest filesystem containing a
// single core type, "fixture-no-schema", with no accompanying JSON Schema
// file. Tests exercising LoadCore's "declared without schema" registration
// path use this instead of coupling to whichever real manifest entry
// happens to lack a schema at the time — that coupling is fragile by
// construction (Phase 6 removed the last two real examples, todo-list and
// plan-review; see manifest/envelopes.yaml).
func fixtureManifestFS() fstest.MapFS {
	return fstest.MapFS{
		"manifest/envelopes.yaml": &fstest.MapFile{
			Data: []byte("core:\n  - type: fixture-no-schema\n"),
		},
	}
}

// loadFixtureRegistry builds a Registry from fixtureManifestFS via the
// public WithManifestFS test/fixture hook (registry.go).
func loadFixtureRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := LoadCore(context.Background(), WithManifestFS(fixtureManifestFS()))
	if err != nil {
		t.Fatalf("LoadCore(fixture): %v", err)
	}
	return r
}

func TestLoadCore_seedsCoreCatalog(t *testing.T) {
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatalf("LoadCore: %v", err)
	}
	if r.Len() == 0 {
		t.Fatal("registry empty after LoadCore")
	}

	mustHave := []string{
		"info-card",
		"approval-card",
		"diff-card",
		"table-card",
		"document-viewer",
		"session-task", // declared without a frontend component
	}
	for _, name := range mustHave {
		spec, ok := r.Lookup(name)
		if !ok {
			t.Errorf("expected core type %q to be registered", name)
			continue
		}
		if spec.Source != TypeSourceCore {
			t.Errorf("type %q: source=%v, want core", name, spec.Source)
		}
		if spec.Name != name {
			t.Errorf("type %q: spec.Name=%q", name, spec.Name)
		}
	}
}

func TestLoadCore_typesWithoutSchemaHaveNilDataSchema(t *testing.T) {
	r := loadFixtureRegistry(t)
	spec, ok := r.Lookup("fixture-no-schema")
	if !ok {
		t.Fatal("fixture-no-schema not registered")
	}
	if spec.DataSchema != nil {
		t.Error("fixture-no-schema has no schema file; DataSchema should be nil")
	}
}

func TestLoadCore_typesWithSchemaCompile(t *testing.T) {
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := r.Lookup("info-card")
	if !ok {
		t.Fatal("info-card not registered")
	}
	if spec.DataSchema == nil {
		t.Error("info-card has a schema file; DataSchema should be non-nil")
	}
}

// TestLoadCore_exposesNoPresentationMetadata replaces the former
// TestLoadCore_uiMetadataPreserved, which asserted that `info-card` resolved to
// components/chat/envelopes/primitives/InfoCard. That assertion pinned the
// violation CW-20260910-0113 removed: a wire contract library naming a path
// inside one host's source tree. Core types now expose no component binding
// through any surface, and this asserts it for every one of them rather than
// for the single type the old test spot-checked.
func TestLoadCore_exposesNoPresentationMetadata(t *testing.T) {
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range r.All() {
		for _, banned := range []string{"component", "export", "props"} {
			if got, found := spec.UIMetadata[banned]; found {
				t.Errorf("core type %q exposes UIMetadata[%s] = %v; component bindings belong to the host",
					spec.Name, banned, got)
			}
		}
		if spec.TypeScript.Import.Component != "" {
			t.Errorf("core type %q exposes TypeScript.Import.Component = %q",
				spec.Name, spec.TypeScript.Import.Component)
		}
		if spec.TypeScript.Import.Export != "" {
			t.Errorf("core type %q exposes TypeScript.Import.Export = %q",
				spec.Name, spec.TypeScript.Import.Export)
		}
		if spec.TypeScript.Import.Props != "" {
			t.Errorf("core type %q exposes TypeScript.Import.Props = %q",
				spec.Name, spec.TypeScript.Import.Props)
		}
	}
}

func TestRegistry_AllSorted(t *testing.T) {
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	specs := r.All()
	for i := 1; i < len(specs); i++ {
		if specs[i-1].Name > specs[i].Name {
			t.Fatalf("All() not sorted: %q before %q", specs[i-1].Name, specs[i].Name)
		}
	}
	if len(specs) != r.Len() {
		t.Errorf("All() returned %d, Len()=%d", len(specs), r.Len())
	}
}

func TestRegistry_Has(t *testing.T) {
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Has("info-card") {
		t.Error("Has(info-card) = false")
	}
	if r.Has("does-not-exist") {
		t.Error("Has(does-not-exist) = true")
	}
}

func TestDefault_returnsSharedRegistry(t *testing.T) {
	a, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("Default() should return the same registry on each call")
	}
	if !a.Has("info-card") {
		t.Error("Default registry should be loaded")
	}
}
