package codegen_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	envelopes "github.com/hollis-labs/go-envelopes"
	"github.com/hollis-labs/go-envelopes/codegen"
)

func TestTypeScript_generatesDataTypesAndImportMetadata(t *testing.T) {
	registry, err := envelopes.LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	first, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("TypeScript generation is not deterministic")
	}
	output := string(first)
	for _, required := range []string{
		"Source: " + envelopes.ModulePath + "@",
		"export interface InfoCardData",
		`variant?: "info" | "success" | "warning" | "danger";`,
		`status: "pending" | "in_progress" | "completed" | "failed" | "canceled" | "cancelled";`,
		"export interface EnvelopeDataMap",
		"export const ENVELOPE_IMPORT_METADATA",
	} {
		if !strings.Contains(output, required) {
			t.Fatalf("generated output missing %q", required)
		}
	}
	// A core-only catalog emits an EMPTY import map. The generator used to
	// write 17 entries of host filesystem paths here — under a comment
	// reading "Host-neutral component import metadata" — which is the
	// coupling CW-20260910-0113 removed. The declaration itself stays: a
	// plugin may still supply import metadata for its own host, and the
	// consumer-facing shape should not change shape based on whether any
	// plugin happens to be registered.
	if strings.Contains(output, "components/chat/envelopes") {
		t.Fatal("core catalog emitted host component paths into the TypeScript output")
	}
	if !strings.Contains(output, "export const ENVELOPE_IMPORT_METADATA = {\n} as const") {
		t.Fatalf("core-only import metadata map is not empty:\n%s",
			output[strings.Index(output, "export const ENVELOPE_IMPORT_METADATA"):])
	}
	if !strings.Contains(output, `Use "canceled" for new payloads.`) {
		t.Fatal("generated session-task status does not document the canonical spelling")
	}
	// The core catalog ships no unregistered schema as of CW-20260910-0114, so
	// opting in changes nothing for it. The opt-in mechanism itself is covered
	// by TestTypeScript_compatibilitySchemasRequireOptIn against a synthetic
	// manifest; this asserts the shipped catalog has nothing to opt into.
	withCompatibility, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{IncludeUnregisteredSchemas: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(withCompatibility) != output {
		t.Fatal("core catalog still ships an unregistered compatibility schema")
	}
}

// TestTypeScript_compatibilitySchemasRequireOptIn covers the
// IncludeUnregisteredSchemas contract without depending on a stray schema
// shipping in the module forever. It previously asserted against KbResultData,
// one of the five app-specific schemas CW-20260910-0114 removed.
func TestTypeScript_compatibilitySchemasRequireOptIn(t *testing.T) {
	manifestFS := fstest.MapFS{
		"manifest/envelopes.yaml": &fstest.MapFile{
			Data: []byte("core:\n  - type: fixture-card\n"),
		},
		"manifest/schemas/fixture-card.schema.json": &fstest.MapFile{
			Data: []byte(`{"$id":"fixture-card.schema.json","title":"Fixture Card","type":"object","properties":{"a":{"type":"string"}}}`),
		},
		"manifest/schemas/fixture-compat.schema.json": &fstest.MapFile{
			Data: []byte(`{"$id":"fixture-compat.schema.json","title":"Fixture Compat","type":"object","properties":{"b":{"type":"string"}}}`),
		},
	}
	registry, err := envelopes.LoadCore(context.Background(), envelopes.WithManifestFS(manifestFS))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	withoutOptIn, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withoutOptIn), "FixtureCompatData") {
		t.Fatal("unregistered compatibility schema emitted without opt-in")
	}
	if !strings.Contains(string(withoutOptIn), "FixtureCardData") {
		t.Fatal("registered type missing from generated output")
	}
	withOptIn, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{IncludeUnregisteredSchemas: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withOptIn), "FixtureCompatData") {
		t.Fatal("opted-in compatibility schema was not generated")
	}
}

func TestTypeScript_pluginTypeFollowsSameGenerationContract(t *testing.T) {
	registry := envelopes.NewRegistry()
	manifest := []byte(`type: demo.calendar-pick
ui:
  component: cards/Calendar
  export: Calendar
  props: envelope
`)
	schema := []byte(`{
  "type":"object",
  "$defs":{"slot":{"type":"object","properties":{"label":{"type":"string"}},"required":["label"]}},
  "properties":{"slots":{"type":"array","items":{"$ref":"#/$defs/slot"}}},
  "required":["slots"]
}`)
	if err := registry.RegisterTypeFromManifest(manifest, schema, "demo"); err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	output, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	generated := string(output)
	for _, required := range []string{
		"export interface DemoCalendarPickSlot",
		"export interface DemoCalendarPickData",
		"slots: DemoCalendarPickSlot[];",
		`"demo.calendar-pick": { component: "cards/Calendar", export: "Calendar", source: "plugin", props: "envelope", pluginId: "demo" }`,
	} {
		if !strings.Contains(generated, required) {
			t.Fatalf("plugin output missing %q\n%s", required, generated)
		}
	}
}

func TestTypeScript_schemaLessPluginGetsUnknownDataType(t *testing.T) {
	registry := envelopes.NewRegistry()
	if err := registry.RegisterType(envelopes.TypeSpec{Name: "1demo.notice", PluginID: "1demo"}); err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	output, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	generated := string(output)
	if !strings.Contains(generated, "export type Envelope1demoNoticeData = unknown;") {
		t.Fatalf("schema-less plugin type missing or invalid identifier:\n%s", generated)
	}
	if !strings.Contains(generated, `"1demo.notice": Envelope1demoNoticeData;`) {
		t.Fatalf("schema-less plugin data map missing:\n%s", generated)
	}
}

func TestTypeScript_rejectsIdentifierCollisions(t *testing.T) {
	catalog := envelopes.Catalog{
		Types: []envelopes.CatalogType{
			{Name: "demo.a-b", TypeScript: envelopes.TypeScriptMetadata{DataType: "DemoABData"}},
			{Name: "demo.a.b", TypeScript: envelopes.TypeScriptMetadata{DataType: "DemoABData"}},
		},
	}
	if _, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{}); err == nil {
		t.Fatal("TypeScript accepted colliding generated identifiers")
	}
}

func TestTypeScript_supportsBooleanSchemas(t *testing.T) {
	registry := envelopes.NewRegistry()
	if err := registry.RegisterTypeFromManifest([]byte("type: demo.any\n"), []byte("true"), "demo"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterTypeFromManifest([]byte("type: demo.never\n"), []byte("false"), "demo"); err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	output, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	generated := string(output)
	if !strings.Contains(generated, "export type DemoAnyData = unknown;") {
		t.Fatalf("true schema output missing:\n%s", generated)
	}
	if !strings.Contains(generated, "export type DemoNeverData = never;") {
		t.Fatalf("false schema output missing:\n%s", generated)
	}
}

func TestTypeScript_supportsNestedBooleanSchemasAndOpenObjects(t *testing.T) {
	registry := envelopes.NewRegistry()
	schema := []byte(`{
  "type": "object",
  "properties": {
    "open": {"type": "object"},
    "openWithKnown": {"type": "object", "properties": {"known": {"type": "string"}}},
    "anything": true,
    "impossible": false,
    "neverItems": {"type": "array", "items": false},
    "stringOrNever": {"oneOf": [{"type": "string"}, false]}
  },
  "additionalProperties": false
}`)
	if err := registry.RegisterTypeFromManifest([]byte("type: demo.shapes\n"), schema, "demo"); err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	output, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	generated := string(output)
	for _, required := range []string{
		"open?: Record<string, unknown>;",
		"openWithKnown?: {",
		"anything?: unknown;",
		"impossible?: never;",
		"neverItems?: never[];",
		"stringOrNever?: string | never;",
	} {
		if !strings.Contains(generated, required) {
			t.Fatalf("nested schema output missing %q:\n%s", required, generated)
		}
	}

	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("tsc is not installed; string-level contract assertions passed")
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "generated.ts"), output, 0o644); err != nil {
		t.Fatal(err)
	}
	usage := `import type { DemoShapesData } from "./generated";

const valid: DemoShapesData = {
  open: { arbitrary: 42 },
  openWithKnown: { known: "yes", arbitrary: 42 },
  anything: { nested: true },
  neverItems: [],
  stringOrNever: "accepted",
};

// @ts-expect-error A false property schema rejects every supplied value.
const invalid: DemoShapesData = { impossible: "must-not-compile" };

void valid;
void invalid;
`
	if err := os.WriteFile(filepath.Join(directory, "usage.ts"), []byte(usage), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(tsc, "--noEmit", "--strict", "--skipLibCheck", "--target", "ES2022", "usage.ts")
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated TypeScript semantic check: %v\n%s", err, output)
	}
}

func TestTypeScript_notTrueIsNeverAcrossNestedPositions(t *testing.T) {
	registry := envelopes.NewRegistry()
	if err := registry.RegisterTypeFromManifest(
		[]byte("type: demo.not-root\n"),
		[]byte(`{"not":true}`),
		"demo",
	); err != nil {
		t.Fatal(err)
	}
	nestedSchema := []byte(`{
  "type": "object",
  "$defs": {
    "impossible": {"not": true}
  },
  "properties": {
    "direct": {"not": true},
    "withSiblings": {"not": true, "type": "object", "properties": {"ignored": {"type": "string"}}},
    "list": {"type": "array", "items": {"not": true}},
    "referenced": {"$ref": "#/$defs/impossible"},
    "allowed": {"not": false}
  },
  "additionalProperties": false
}`)
	if err := registry.RegisterTypeFromManifest(
		[]byte("type: demo.not-nested\n"),
		nestedSchema,
		"demo",
	); err != nil {
		t.Fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		t.Fatal(err)
	}
	output, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	generated := string(output)
	for _, required := range []string{
		"export type DemoNotRootData = never;",
		"export type DemoNotNestedImpossible = never;",
		"direct?: never;",
		"withSiblings?: never;",
		"list?: never[];",
		"referenced?: DemoNotNestedImpossible;",
		"allowed?: unknown;",
	} {
		if !strings.Contains(generated, required) {
			t.Fatalf("not applicator output missing %q:\n%s", required, generated)
		}
	}

	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("tsc is not installed; string-level contract assertions passed")
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "generated.ts"), output, 0o644); err != nil {
		t.Fatal(err)
	}
	usage := `import type { DemoNotNestedData, DemoNotRootData } from "./generated";

const valid: DemoNotNestedData = { allowed: { anything: true }, list: [] };

// @ts-expect-error A not:true property rejects every supplied value.
const invalidNested: DemoNotNestedData = { direct: "must-not-compile" };

// @ts-expect-error not:true remains impossible when sibling keywords exist.
const invalidWithSiblings: DemoNotNestedData = { withSiblings: {} };

// @ts-expect-error Arrays whose item schema is not:true cannot contain values.
const invalidList: DemoNotNestedData = { list: ["must-not-compile"] };

// @ts-expect-error A $ref to a not:true $defs entry rejects every value.
const invalidReference: DemoNotNestedData = { referenced: "must-not-compile" };

// @ts-expect-error A root not:true schema rejects every value.
const invalidRoot: DemoNotRootData = {};

void valid;
void invalidNested;
void invalidWithSiblings;
void invalidList;
void invalidReference;
void invalidRoot;
`
	if err := os.WriteFile(filepath.Join(directory, "usage.ts"), []byte(usage), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(tsc, "--noEmit", "--strict", "--skipLibCheck", "--target", "ES2022", "usage.ts")
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("not applicator TypeScript semantic check: %v\n%s", err, output)
	}
}
