// Package main demonstrates the minimal go-envelopes usage pattern:
// load the embedded core registry and run an envelope through the
// validator.
//
// Run from the repo root:
//
//	go run ./examples/validate
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	envelopes "github.com/hollis-labs/go-envelopes"
)

func main() {
	// LoadCore parses the embedded YAML manifest, compiles every
	// per-type JSON Schema present on disk, and seeds the registry
	// with TypeSourceCore entries.
	reg, err := envelopes.LoadCore(context.Background())
	if err != nil {
		log.Fatalf("load core registry: %v", err)
	}
	fmt.Printf("loaded %d core types\n", reg.Len())

	// Known-good envelope. ProtocolVersion is the constant the
	// library was built against; bump only when the wire-format
	// major version changes.
	good := &envelopes.Envelope{
		V:    envelopes.ProtocolVersion,
		ID:   "env_demo_1",
		Type: "info-card",
		Data: map[string]any{
			"title": "Hello",
			"body":  "world",
		},
	}
	if err := reg.ValidateEnvelope(good); err != nil {
		log.Fatalf("known-good envelope failed validation: %v", err)
	}
	fmt.Println("known-good info-card validated OK")

	// New responses use the canonical US-English cancellation vocabulary.
	// ValidateResponse still recognizes v0.2-era "cancelled" input throughout
	// v0.4.x; call ResponseStatus.Canonical before re-emitting stored input.
	response := &envelopes.Response{
		V:          envelopes.ProtocolVersion,
		EnvelopeID: good.ID,
		Kind:       envelopes.ResponseKindAck,
		Status:     envelopes.ResponseStatusCanceled,
	}
	if err := reg.ValidateResponse(good.Type, response); err != nil {
		log.Fatalf("known-good response failed validation: %v", err)
	}
	fmt.Println("known-good canceled response validated OK")

	// Known-bad envelope: unknown type. Surfaces ErrUnknownType so
	// callers can branch on the typed error.
	bad := &envelopes.Envelope{
		V:    envelopes.ProtocolVersion,
		ID:   "env_demo_2",
		Type: "no-such-type",
		Data: map[string]any{},
	}
	err = reg.ValidateEnvelope(bad)
	switch {
	case err == nil:
		log.Fatal("expected unknown-type error, got nil")
	case errors.Is(err, envelopes.ErrUnknownType):
		fmt.Printf("known-bad envelope rejected with ErrUnknownType: %v\n", err)
	default:
		log.Fatalf("unexpected error kind: %v", err)
	}
}
