package envelopes

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// coreSupportCoveringEverything returns a TypeSupport that accounts for every
// registered core type, with the two ADR 0005 boundary violations excluded by
// name — the exact shape a host like Tangent would declare.
func coreSupportCoveringEverything(t *testing.T, r *Registry) TypeSupport {
	t.Helper()
	excludes := map[string]string{
		"subagent-spawn-approval": "ADR 0005 — not a session manager",
		"chat-loop-terminated":    "ADR 0005 — not an agent launcher",
	}
	var supports []string
	for _, spec := range r.All() {
		if _, skipped := excludes[spec.Name]; !skipped {
			supports = append(supports, spec.Name)
		}
	}
	return TypeSupport{Supports: supports, Excludes: excludes}
}

func TestCheckSupport_completeDeclarationPasses(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.CheckSupport(coreSupportCoveringEverything(t, registry)); err != nil {
		t.Fatalf("complete declaration rejected: %v", err)
	}
}

// TestCheckSupport_omissionIsNotExclusion is the whole point of the mechanism,
// and the landmine it exists to disarm. Dropping a type from the supported list
// without excluding it by name is indistinguishable from never having heard of
// it, which is how a deliberately-rejected type returns the next time a
// consumer regenerates against a newer catalog. It must be an error, not a
// silent narrowing.
func TestCheckSupport_omissionIsNotExclusion(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	support := coreSupportCoveringEverything(t, registry)
	// Drop subagent-spawn-approval's named exclusion; simply not mentioning it
	// is what a regenerated sweep would produce.
	delete(support.Excludes, "subagent-spawn-approval")

	err = registry.CheckSupport(support)
	if err == nil {
		t.Fatal("omitting a type silently passed; exclusion by omission must be an error")
	}
	var gap *SupportGap
	if !errors.As(err, &gap) {
		t.Fatalf("expected *SupportGap, got %T", err)
	}
	if len(gap.Unclaimed) != 1 || gap.Unclaimed[0] != "subagent-spawn-approval" {
		t.Fatalf("unclaimed = %v, want [subagent-spawn-approval]", gap.Unclaimed)
	}
	if !strings.Contains(gap.Error(), "exclude them by name") {
		t.Errorf("error message does not tell the caller what to do: %q", gap.Error())
	}
}

// TestCheckSupport_newCatalogTypeIsUnclaimed simulates the upgrade path: a
// consumer's declaration written against today's catalog must fail when the
// library later adds a type, so adopting it is a decision rather than a default.
func TestCheckSupport_newCatalogTypeIsUnclaimed(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	support := coreSupportCoveringEverything(t, registry)

	if err := registry.RegisterType(TypeSpec{Name: "demo.newcomer", PluginID: "demo"}); err != nil {
		t.Fatal(err)
	}
	err = registry.CheckSupport(support)
	if err == nil {
		t.Fatal("a newly registered type was silently absorbed")
	}
	var gap *SupportGap
	if !errors.As(err, &gap) {
		t.Fatalf("expected *SupportGap, got %T", err)
	}
	if len(gap.Unclaimed) != 1 || gap.Unclaimed[0] != "demo.newcomer" {
		t.Fatalf("unclaimed = %v, want [demo.newcomer]", gap.Unclaimed)
	}
}

func TestCheckSupport_reportsUnknownAndConflictingNames(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	support := coreSupportCoveringEverything(t, registry)
	support.Supports = append(support.Supports, "info-crad") // typo
	support.Excludes["info-card"] = "deliberate conflict"    // also supported

	err = registry.CheckSupport(support)
	if err == nil {
		t.Fatal("expected a gap for the typo and the conflict")
	}
	var gap *SupportGap
	if !errors.As(err, &gap) {
		t.Fatalf("expected *SupportGap, got %T", err)
	}
	if len(gap.Unknown) != 1 || gap.Unknown[0] != "info-crad" {
		t.Errorf("unknown = %v, want [info-crad]", gap.Unknown)
	}
	if len(gap.Conflicting) != 1 || gap.Conflicting[0] != "info-card" {
		t.Errorf("conflicting = %v, want [info-card]", gap.Conflicting)
	}
}

func TestSupportedTypes_excludesUnregisteredAndSorts(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := registry.SupportedTypes(TypeSupport{
		Supports: []string{"metric-card", "info-card", "not-a-real-type", "info-card"},
	})
	if len(got) != 2 || got[0] != "info-card" || got[1] != "metric-card" {
		t.Fatalf("SupportedTypes = %v, want [info-card metric-card]", got)
	}
}
