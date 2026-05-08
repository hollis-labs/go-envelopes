package envelopes

import (
	"errors"
	"sync"
	"testing"
)

func TestRegisterType_validNamespacedName(t *testing.T) {
	r := mustLoad(t)
	spec := TypeSpec{
		Name:        "demo.calendar-pick",
		Version:     "0.1.0",
		Description: "Pick a calendar slot",
		PluginID:    "demo",
	}
	if err := r.RegisterType(spec); err != nil {
		t.Fatalf("RegisterType: %v", err)
	}
	got, ok := r.Lookup("demo.calendar-pick")
	if !ok {
		t.Fatal("type not registered after RegisterType")
	}
	if got.Source != TypeSourcePlugin {
		t.Errorf("Source = %v, want plugin", got.Source)
	}
	if got.PluginID != "demo" {
		t.Errorf("PluginID = %q, want demo", got.PluginID)
	}
	if got.ResponseKind != ResponseKindData {
		t.Errorf("ResponseKind defaulted to %q, want data", got.ResponseKind)
	}
}

func TestRegisterType_rejectsUnnamespacedName(t *testing.T) {
	r := mustLoad(t)
	tests := []string{"calendar-pick", "DEMO.calendar", ".calendar", "demo.", "demo..type", "demo .type"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			err := r.RegisterType(TypeSpec{Name: name, PluginID: "demo"})
			if !errors.Is(err, ErrInvalidName) {
				t.Errorf("name %q: expected ErrInvalidName, got %v", name, err)
			}
		})
	}
}

func TestRegisterType_acceptsMultiSegmentName(t *testing.T) {
	// Plugins may stack a vendor prefix in front of the plugin id
	// ("myorg.myplugin.kind") — the namespaced regex permits any chain
	// of dot-separated kebab segments with at least one dot.
	r := mustLoad(t)
	for _, name := range []string{
		"myorg.myplugin.calendar-pick",
		"a.b.c.d",
		"vendor.plugin1.feature.subkind",
	} {
		t.Run(name, func(t *testing.T) {
			err := r.RegisterType(TypeSpec{Name: name, PluginID: "myplugin"})
			if err != nil {
				t.Errorf("name %q: expected accept, got %v", name, err)
			}
			if !r.Has(name) {
				t.Errorf("name %q: not registered", name)
			}
		})
	}
}

func TestRegisterType_overridesCoreSourceClaim(t *testing.T) {
	// A caller that mistakenly hands TypeSourceCore to the plugin API
	// gets the value silently overwritten with TypeSourcePlugin. This
	// keeps UnregisterType's core-type protection from being corrupted
	// by a misbehaving plugin host.
	r := mustLoad(t)
	if err := r.RegisterType(TypeSpec{Name: "demo.coerced", Source: TypeSourceCore, PluginID: "demo"}); err != nil {
		t.Fatalf("RegisterType: %v", err)
	}
	spec, _ := r.Lookup("demo.coerced")
	if spec.Source != TypeSourcePlugin {
		t.Errorf("Source = %v, want plugin (overwritten)", spec.Source)
	}
}

func TestRegisterType_rejectsCoreNameOverride(t *testing.T) {
	r := mustLoad(t)
	// "info-card" is a core type. The plugin namespace check rejects an
	// un-namespaced name first, which is precisely the desired protection
	// against core-name shadowing.
	err := r.RegisterType(TypeSpec{Name: "info-card", PluginID: "demo"})
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

func TestRegisterType_conflictOnDuplicate(t *testing.T) {
	r := mustLoad(t)
	if err := r.RegisterType(TypeSpec{Name: "demo.dup", PluginID: "demo"}); err != nil {
		t.Fatal(err)
	}
	err := r.RegisterType(TypeSpec{Name: "demo.dup", PluginID: "demo"})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestUnregisterType_removesPluginType(t *testing.T) {
	r := mustLoad(t)
	if err := r.RegisterType(TypeSpec{Name: "demo.gone", PluginID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := r.UnregisterType("demo.gone"); err != nil {
		t.Fatalf("UnregisterType: %v", err)
	}
	if r.Has("demo.gone") {
		t.Error("type still registered after UnregisterType")
	}
}

func TestUnregisterType_protectsCore(t *testing.T) {
	r := mustLoad(t)
	err := r.UnregisterType("info-card")
	if !errors.Is(err, ErrCoreTypeProtected) {
		t.Errorf("expected ErrCoreTypeProtected, got %v", err)
	}
	if !r.Has("info-card") {
		t.Error("core type was removed despite protection")
	}
}

func TestUnregisterType_unknownReturnsErrUnknownType(t *testing.T) {
	r := mustLoad(t)
	err := r.UnregisterType("never.existed")
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("expected ErrUnknownType, got %v", err)
	}
}

func TestUnregisterPlugin_dropsAllOwnedTypes(t *testing.T) {
	r := mustLoad(t)
	for _, name := range []string{"demo.a", "demo.b", "other.c"} {
		pluginID := "demo"
		if name[:5] != "demo." {
			pluginID = "other"
		}
		if err := r.RegisterType(TypeSpec{Name: name, PluginID: pluginID}); err != nil {
			t.Fatal(err)
		}
	}
	n := r.UnregisterPlugin("demo")
	if n != 2 {
		t.Errorf("UnregisterPlugin removed %d, want 2", n)
	}
	if r.Has("demo.a") || r.Has("demo.b") {
		t.Error("demo types still registered")
	}
	if !r.Has("other.c") {
		t.Error("other.c was dropped despite different plugin id")
	}
}

func TestRegisterTypeFromManifest_yamlAndSchema(t *testing.T) {
	r := mustLoad(t)
	manifest := []byte(`
type: calendar-pick
version: 0.2.0
description: Pick a calendar slot
ui:
  component: components/CalendarPick
  export: CalendarPick
`)
	schema := []byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"required": ["slot"],
		"properties": {
			"slot": {"type": "string"}
		},
		"additionalProperties": false
	}`)
	if err := r.RegisterTypeFromManifest(manifest, schema, "demo"); err != nil {
		t.Fatalf("RegisterTypeFromManifest: %v", err)
	}
	spec, ok := r.Lookup("demo.calendar-pick")
	if !ok {
		t.Fatal("expected demo.calendar-pick to be registered")
	}
	if spec.Version != "0.2.0" {
		t.Errorf("Version = %q, want 0.2.0", spec.Version)
	}
	if spec.PluginID != "demo" {
		t.Errorf("PluginID = %q", spec.PluginID)
	}
	if spec.DataSchema == nil {
		t.Error("DataSchema is nil; schema should have been compiled")
	}

	good := &Envelope{V: 1, ID: "1", Type: "demo.calendar-pick", Data: map[string]any{"slot": "9am"}}
	if err := r.ValidateEnvelope(good); err != nil {
		t.Errorf("ValidateEnvelope (good): %v", err)
	}
	bad := &Envelope{V: 1, ID: "1", Type: "demo.calendar-pick", Data: map[string]any{"unknown": true}}
	if err := r.ValidateEnvelope(bad); err == nil {
		t.Error("ValidateEnvelope (bad): expected schema validation failure")
	}
}

func TestRegisterTypeFromManifest_jsonFallback(t *testing.T) {
	r := mustLoad(t)
	manifest := []byte(`{"type": "json-form", "version": "0.1.0"}`)
	if err := r.RegisterTypeFromManifest(manifest, nil, "demo"); err != nil {
		t.Fatalf("RegisterTypeFromManifest: %v", err)
	}
	if !r.Has("demo.json-form") {
		t.Error("expected demo.json-form to be registered")
	}
}

// TestRegistry_concurrentAccess exercises the RWMutex by spinning concurrent
// readers and writers. -race catches misuse.
func TestRegistry_concurrentAccess(t *testing.T) {
	r := mustLoad(t)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Has("info-card")
			_ = r.All()
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "race.t" + string(rune('a'+i))
			_ = r.RegisterType(TypeSpec{Name: name, PluginID: "race"})
			_ = r.UnregisterType(name)
		}(i)
	}
	wg.Wait()
}
