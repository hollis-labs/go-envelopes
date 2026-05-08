package envelopes

import (
	"context"
	"testing"
)

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
		"question-form",
		"session-task",          // declared without schema (no schema file)
		"message-request",       // declared without component or schema
		"chat-loop-budget-soft-warning",
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
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := r.Lookup("message-request")
	if !ok {
		t.Fatal("message-request not registered")
	}
	if spec.DataSchema != nil {
		t.Error("message-request has no schema file; DataSchema should be nil")
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

func TestLoadCore_uiMetadataPreserved(t *testing.T) {
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := r.Lookup("info-card")
	if !ok {
		t.Fatal()
	}
	if got, want := spec.UIMetadata["component"], "components/chat/envelopes/primitives/InfoCard"; got != want {
		t.Errorf("UIMetadata[component] = %v, want %v", got, want)
	}
	if got, want := spec.UIMetadata["export"], "InfoCard"; got != want {
		t.Errorf("UIMetadata[export] = %v, want %v", got, want)
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
