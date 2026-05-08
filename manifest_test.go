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

	// Spot-check entries: at least one type with a component (info-card)
	// and at least one without (message-request — backend-only).
	var seenWithComponent, seenWithoutComponent bool
	for _, e := range m.Core {
		if e.Type == "info-card" && e.Component != "" && e.Export == "InfoCard" {
			seenWithComponent = true
		}
		if e.Type == "message-request" && e.Component == "" {
			seenWithoutComponent = true
		}
	}
	if !seenWithComponent {
		t.Error("expected info-card entry with component=...InfoCard, export=InfoCard")
	}
	if !seenWithoutComponent {
		t.Error("expected message-request entry with no component (backend-only)")
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
