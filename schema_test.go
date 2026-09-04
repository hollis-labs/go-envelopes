package envelopes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSchemaDocument_exposesAnnotationsWithoutRawParsing(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := registry.Lookup("info-card")
	if !ok || spec.DataSchemaDocument == nil {
		t.Fatal("info-card schema document is not exposed")
	}

	root := spec.DataSchemaDocument.Metadata()
	if got := root.Custom["default_render_target"]; got != "bottom_chat_drawer" {
		t.Fatalf("default_render_target = %v, want bottom_chat_drawer", got)
	}
	if root.AdditionalPropertiesAllowed == nil || *root.AdditionalPropertiesAllowed {
		t.Fatal("root additionalProperties=false annotation not exposed")
	}

	field, ok := spec.DataSchemaDocument.MetadataAtInstancePath("/variant")
	if !ok {
		t.Fatal("variant metadata not found")
	}
	if got, want := len(field.Enum), 4; got != want {
		t.Fatalf("variant enum length = %d, want %d", got, want)
	}
	if field.Description == "" {
		t.Fatal("variant description is empty")
	}

	listSpec, ok := registry.Lookup("list-card")
	if !ok || listSpec.DataSchemaDocument == nil {
		t.Fatal("list-card schema document is not exposed")
	}
	nested, ok := listSpec.DataSchemaDocument.MetadataAtInstancePath("/items/0/label")
	if !ok || len(nested.Types) != 1 || nested.Types[0] != "string" {
		t.Fatalf("nested array metadata = %#v, found=%v", nested, ok)
	}

	// Returned metadata is defensive: mutation must not affect a later read.
	root.Custom["default_render_target"] = "mutated"
	if got := spec.DataSchemaDocument.Metadata().Custom["default_render_target"]; got != "bottom_chat_drawer" {
		t.Fatalf("schema metadata was mutable through accessor: %v", got)
	}
}

func TestValidateEnvelope_structuredFailures(t *testing.T) {
	registry, err := LoadCore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	envelope := &Envelope{
		V:    ProtocolVersion,
		ID:   "structured_failure",
		Type: "info-card",
		Data: map[string]any{
			"title":    float64(42),
			"surprise": true,
		},
	}
	err = registry.ValidateEnvelope(envelope)
	if !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("ValidateEnvelope error = %v, want ErrSchemaValidation", err)
	}
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	failures := validationError.Details()
	if len(failures) != 3 {
		t.Fatalf("failure count = %d, want 3: %#v", len(failures), failures)
	}

	byKeyword := make(map[string]ValidationFailure, len(failures))
	for _, failure := range failures {
		if failure.Keyword == "" || failure.SchemaPath == "" {
			t.Fatalf("failure lacks keyword/schema path: %#v", failure)
		}
		byKeyword[failure.Keyword] = failure
	}
	required := byKeyword["required"]
	if required.InstancePath != "" || required.SchemaPath != "/required" {
		t.Fatalf("required paths = instance %q schema %q", required.InstancePath, required.SchemaPath)
	}
	if got := required.Expected.([]string); len(got) != 1 || got[0] != "body" {
		t.Fatalf("required expected = %#v, want [body]", got)
	}
	typeFailure := byKeyword["type"]
	if typeFailure.InstancePath != "/title" || typeFailure.SchemaPath != "/properties/title/type" {
		t.Fatalf("type paths = instance %q schema %q", typeFailure.InstancePath, typeFailure.SchemaPath)
	}
	if typeFailure.Actual == nil || typeFailure.Actual.Type != "number" {
		t.Fatalf("type actual = %#v, want number summary", typeFailure.Actual)
	}
	additional := byKeyword["additionalProperties"]
	if additional.Metadata == nil || len(additional.Metadata.Properties) == 0 {
		t.Fatalf("additionalProperties lacks allowed-property metadata: %#v", additional)
	}
	if strings.Contains(additional.Message, "surprise") {
		t.Fatalf("additionalProperties message echoed payload key: %q", additional.Message)
	}
	if additional.Actual == nil || additional.Actual.Value != nil {
		t.Fatalf("additionalProperties actual must be bounded: %#v", additional.Actual)
	}
}

func TestPluginSchema_usesStructuredContractAndAnnotations(t *testing.T) {
	registry := NewRegistry()
	manifest := []byte(`
type: calendar-pick
version: 2.1.0
ui:
  component: cards/CalendarPick
  export: CalendarPick
`)
	schema := []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["slot"],
  "properties": {"slot": {"type": "string", "enum": ["morning", "evening"]}},
  "x-plugin-panel": "calendar",
  "additionalProperties": false
}`)
	if err := registry.RegisterTypeFromManifest(manifest, schema, "demo"); err != nil {
		t.Fatal(err)
	}
	spec, ok := registry.Lookup("demo.calendar-pick")
	if !ok || spec.DataSchemaDocument == nil {
		t.Fatal("plugin type/schema missing from registry")
	}
	if got := spec.DataSchemaDocument.Metadata().Custom["x-plugin-panel"]; got != "calendar" {
		t.Fatalf("plugin annotation = %v, want calendar", got)
	}
	if spec.TypeScript.Import.Component != "cards/CalendarPick" || spec.TypeScript.DataType != "DemoCalendarPickData" {
		t.Fatalf("plugin TypeScript metadata = %#v", spec.TypeScript)
	}

	err := registry.ValidateEnvelope(&Envelope{
		V: ProtocolVersion, ID: "plugin_bad", Type: "demo.calendar-pick",
		Data: map[string]any{"slot": "afternoon"},
	})
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("plugin validation error = %v", err)
	}
	failures := validationError.Details()
	if len(failures) != 1 || failures[0].Keyword != "enum" {
		t.Fatalf("plugin failures = %#v", failures)
	}
	if got := failures[0].Expected.([]any); len(got) != 2 {
		t.Fatalf("plugin enum expected = %#v", got)
	}
	if failures[0].Actual == nil || failures[0].Actual.Type != "string" || failures[0].Actual.Value != nil {
		t.Fatalf("plugin actual must summarize, not echo, payload: %#v", failures[0].Actual)
	}
}

func TestValidationError_loggingSurfacesDoNotLeakPayloadValues(t *testing.T) {
	registry := NewRegistry()
	schema := []byte(`{
  "type": "object",
  "required": ["token"],
  "properties": {"token": {"type": "string", "pattern": "^public-[a-z]+$"}},
  "additionalProperties": false
}`)
	if err := registry.RegisterTypeFromManifest([]byte("type: demo.secret\n"), schema, "demo"); err != nil {
		t.Fatal(err)
	}
	secret := "SECRET-token-12345"
	err := registry.ValidateEnvelope(&Envelope{
		V: ProtocolVersion, ID: "secret", Type: "demo.secret",
		Data: map[string]any{"token": secret},
	})
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("validation error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Error leaked raw payload value: %q", err.Error())
	}
	details := validationError.Details()
	if len(details) != 1 || details[0].Message != "value does not match required pattern" {
		t.Fatalf("safe validation details = %#v", details)
	}
	if strings.Contains(details[0].Message, secret) {
		t.Fatalf("failure message leaked raw payload value: %q", details[0].Message)
	}
	raw, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("marshaled validation details leaked raw payload value: %s", raw)
	}

	propertySchema := []byte(`{
  "type": "object",
  "propertyNames": {"pattern": "^[a-z]+$"}
}`)
	if err := registry.RegisterTypeFromManifest([]byte("type: demo.secret-property\n"), propertySchema, "demo"); err != nil {
		t.Fatal(err)
	}
	secretProperty := "SECRET_PROPERTY_12345"
	err = registry.ValidateEnvelope(&Envelope{
		V: ProtocolVersion, ID: "secret-property", Type: "demo.secret-property",
		Data: map[string]any{secretProperty: true},
	})
	if !errors.As(err, &validationError) {
		t.Fatalf("property-name validation error = %v", err)
	}
	propertyValidationErr := err
	propertyDetails, marshalErr := json.Marshal(validationError.Details())
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(propertyValidationErr.Error(), secretProperty) || strings.Contains(string(propertyDetails), secretProperty) {
		t.Fatalf("validation diagnostics leaked raw property name: error=%q details=%s", propertyValidationErr, propertyDetails)
	}
}
