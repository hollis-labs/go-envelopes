package envelopes

import (
	"io/fs"
	"strings"
	"testing"
)

func TestParseManifest_canonical(t *testing.T) {
	data, err := fs.ReadFile(embeddedManifest, "manifest/envelopes.yaml")
	if err != nil {
		t.Fatalf("read embedded manifest: %v", err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Core) == 0 {
		t.Fatal("core list is empty; manifest seed not extracted")
	}

	// Spot-check that known core types are still declared. The manifest is
	// the wire contract; losing an entry silently drops a type from
	// validation.
	declared := make(map[string]bool, len(m.Core))
	for _, e := range m.Core {
		declared[e.Type] = true
	}
	for _, want := range []string{"info-card", "session-task", "approval-card"} {
		if !declared[want] {
			t.Errorf("core manifest is missing envelope type %q", want)
		}
	}
}

// TestParseManifest_carriesNoPresentationMetadata is the regression gate for
// CW-20260910-0113. The core manifest asserted `component`, `export` and
// `props` on 17 of its 18 entries, each naming a path, symbol or prop
// convention inside one host's source tree — an appearance claim a wire
// contract library has no authority to make. They were removed in v0.5.0.
//
// This asserts the absence at the YAML level rather than the struct level:
// the fields are gone from ManifestEntry, so a reintroduced `component:` key
// now lands in Extra instead of failing to compile. Extra is exactly where a
// regenerating sweep would put it back unnoticed, so that is where to look.
func TestParseManifest_carriesNoPresentationMetadata(t *testing.T) {
	data, err := fs.ReadFile(embeddedManifest, "manifest/envelopes.yaml")
	if err != nil {
		t.Fatalf("read embedded manifest: %v", err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, e := range m.Core {
		for _, banned := range []string{"component", "export", "props"} {
			if _, found := e.Extra[banned]; found {
				t.Errorf("core type %q reintroduced presentation field %q; "+
					"component bindings belong to the host, not the wire manifest", e.Type, banned)
			}
		}
		if metadata := uiMetadataForEntry(e); metadata != nil {
			for _, banned := range []string{"component", "export", "props"} {
				if _, found := metadata[banned]; found {
					t.Errorf("core type %q exports presentation metadata %q", e.Type, banned)
				}
			}
		}
	}
}

// TestManifest_metaschemaRejectsPresentationFieldsByName pins the second half
// of the fence. Deleting the values while leaving the metaschema permissive
// would let the next sweep re-add them silently; `additionalProperties: false`
// plus the removed property definitions means they are refused BY NAME.
func TestManifest_metaschemaRejectsPresentationFieldsByName(t *testing.T) {
	raw, err := fs.ReadFile(embeddedManifest, "manifest/envelopes.schema.json")
	if err != nil {
		t.Fatalf("read metaschema: %v", err)
	}
	document, err := NewSchemaDocument("envelopes-v1.schema.json", raw)
	if err != nil {
		t.Fatalf("parse metaschema: %v", err)
	}
	compiled, err := compileSchemaDocument(document)
	if err != nil {
		t.Fatalf("compile metaschema: %v", err)
	}
	for _, banned := range []string{"component", "export", "props"} {
		instance := map[string]any{
			"core": []any{map[string]any{"type": "demo-card", banned: "components/chat/envelopes/DemoCard"}},
		}
		if err := compiled.Validate(instance); err == nil {
			t.Errorf("metaschema accepted presentation field %q; it must be rejected by name", banned)
		}
	}
	valid := map[string]any{"core": []any{map[string]any{"type": "demo-card"}}}
	if err := compiled.Validate(valid); err != nil {
		t.Fatalf("metaschema rejected a wire-only entry: %v", err)
	}
}

func TestParseManifest_rejectsBadYAML(t *testing.T) {
	_, err := ParseManifest([]byte("not: valid: yaml: ["))
	if err == nil {
		t.Fatal("expected parse error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "parse manifest") {
		t.Errorf("error should mention parse manifest, got %q", err)
	}
}

func TestParseManifest_preservesUnknownGeneratorMetadata(t *testing.T) {
	manifest, err := ParseManifest([]byte("core:\n  - type: demo-card\n    x-loader: eager\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Core[0].Extra["x-loader"]; got != "eager" {
		t.Fatalf("unknown metadata = %v, want eager", got)
	}
	metadata := importMetadataForEntry(manifest.Core[0])
	if got := metadata.Extra["x-loader"]; got != "eager" {
		t.Fatalf("import metadata extra = %v, want eager", got)
	}
}
