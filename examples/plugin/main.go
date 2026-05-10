// Package main demonstrates registering a plugin-owned envelope type
// at runtime via the extension API, then validating an envelope
// against it.
//
// Run from the repo root:
//
//	go run ./examples/plugin
package main

import (
	"context"
	"fmt"
	"log"

	envelopes "github.com/hollis-labs/go-envelopes"
)

// pluginManifest is what a plugin-SDK consumer would normally embed
// or fetch from a plugin bundle. Field shape matches the YAML in
// manifest/envelopes.yaml so plugin manifests stay symmetrical with
// the core seed.
const pluginManifest = `
type: example-plugin.calendar-pick
description: Calendar picker contributed by a plugin.
responseKind: data
ui:
  component: components/CalendarPickCard
  export: CalendarPickCard
`

// pluginSchema is the per-type JSON Schema the plugin ships for its
// envelope payload. Matches the on-disk shape in manifest/schemas/.
const pluginSchema = `{
  "$id": "example-plugin.calendar-pick",
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Calendar Pick",
  "type": "object",
  "required": ["min_date", "max_date"],
  "properties": {
    "min_date": {"type": "string", "format": "date"},
    "max_date": {"type": "string", "format": "date"},
    "default":  {"type": "string", "format": "date"}
  },
  "additionalProperties": false
}`

func main() {
	reg, err := envelopes.LoadCore(context.Background())
	if err != nil {
		log.Fatalf("load core registry: %v", err)
	}

	// RegisterTypeFromManifest is the convenience entry point: parse
	// a manifest fragment + schema fragment together and stitch them
	// into a TypeSpec under the given pluginID. The library forces
	// TypeSource = plugin so UnregisterPlugin can target it cleanly.
	if err := reg.RegisterTypeFromManifest(
		[]byte(pluginManifest),
		[]byte(pluginSchema),
		"example-plugin",
	); err != nil {
		log.Fatalf("register plugin type: %v", err)
	}
	fmt.Printf("registered plugin type; registry now holds %d types\n", reg.Len())

	env := &envelopes.Envelope{
		V:    envelopes.ProtocolVersion,
		ID:   "env_plugin_1",
		Type: "example-plugin.calendar-pick",
		Data: map[string]any{
			"min_date": "2026-01-01",
			"max_date": "2026-12-31",
			"default":  "2026-06-01",
		},
	}
	if err := reg.ValidateEnvelope(env); err != nil {
		log.Fatalf("plugin envelope failed validation: %v", err)
	}
	fmt.Println("plugin envelope validated OK")

	// Plugin lifecycle: unregister all types owned by the plugin in
	// one call (e.g. on plugin shutdown / reload).
	removed := reg.UnregisterPlugin("example-plugin")
	fmt.Printf("unregistered %d plugin type(s); registry now holds %d types\n", removed, reg.Len())
}
