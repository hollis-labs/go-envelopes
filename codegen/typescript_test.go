package codegen_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
		"export interface EnvelopeDataMap",
		"export const ENVELOPE_IMPORT_METADATA",
		`"info-card": { component: "components/chat/envelopes/primitives/InfoCard", export: "InfoCard", source: "core"`,
	} {
		if !strings.Contains(output, required) {
			t.Fatalf("generated output missing %q", required)
		}
	}
	if strings.Contains(output, "KbResultData") {
		t.Fatal("unregistered compatibility schema emitted without opt-in")
	}

	withCompatibility, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{IncludeUnregisteredSchemas: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withCompatibility), "export interface KbResultData") {
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
