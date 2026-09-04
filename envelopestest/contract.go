// Package envelopestest provides a contract test downstream consumers can
// run to verify that their integration with go-envelopes preserves the
// invariants the library guarantees. Use it in your own *_test.go to gate
// regressions when upgrading go-envelopes:
//
//	func TestMyHost_envelopesContract(t *testing.T) {
//	    reg, _ := envelopes.LoadCore(context.Background())
//	    envelopestest.RunContract(t, reg)
//	}
package envelopestest

import (
	"errors"
	"testing"

	envelopes "github.com/hollis-labs/go-envelopes"
)

// RunContract exercises a Registry against the v1 invariants. It assumes
// the registry was loaded from the embedded core manifest; consumers that
// supply a custom manifest filesystem (via WithManifestFS) should still
// satisfy these checks because the contract is about API behavior rather
// than catalog contents.
func RunContract(t *testing.T, reg *envelopes.Registry) {
	t.Helper()
	if reg == nil {
		t.Fatal("envelopestest: nil registry")
	}

	t.Run("validate-envelope-success", func(t *testing.T) {
		t.Helper()
		env := &envelopes.Envelope{
			V:    envelopes.ProtocolVersion,
			ID:   "envelopestest_1",
			Type: "info-card",
			Data: map[string]any{"title": "T", "body": "B"},
		}
		if err := reg.ValidateEnvelope(env); err != nil {
			t.Errorf("known-good info-card failed validation: %v", err)
		}
	})

	t.Run("validate-envelope-unknown-type", func(t *testing.T) {
		t.Helper()
		env := &envelopes.Envelope{
			V:    envelopes.ProtocolVersion,
			ID:   "envelopestest_2",
			Type: "envelopestest.does-not-exist",
		}
		err := reg.ValidateEnvelope(env)
		if !errors.Is(err, envelopes.ErrUnknownType) {
			t.Errorf("expected ErrUnknownType, got %v", err)
		}
	})

	t.Run("plugin-extension-roundtrip", func(t *testing.T) {
		t.Helper()
		const name = "envelopestest.contract-tmp"
		spec := envelopes.TypeSpec{
			Name:     name,
			Version:  "0.0.1",
			PluginID: "envelopestest",
		}
		if err := reg.RegisterType(spec); err != nil {
			t.Fatalf("RegisterType: %v", err)
		}
		if !reg.Has(name) {
			t.Fatal("type not registered after RegisterType")
		}
		if err := reg.UnregisterType(name); err != nil {
			t.Fatalf("UnregisterType: %v", err)
		}
		if reg.Has(name) {
			t.Error("type still registered after UnregisterType")
		}
	})

	t.Run("core-types-protected", func(t *testing.T) {
		t.Helper()
		err := reg.UnregisterType("info-card")
		if !errors.Is(err, envelopes.ErrCoreTypeProtected) {
			t.Errorf("expected ErrCoreTypeProtected, got %v", err)
		}
	})

	t.Run("response-canonical-kinds", func(t *testing.T) {
		t.Helper()
		resp := &envelopes.Response{
			V:          envelopes.ProtocolVersion,
			EnvelopeID: "envelopestest_3",
			Kind:       envelopes.ResponseKindAck,
			Status:     envelopes.ResponseStatusSubmitted,
		}
		if err := reg.ValidateResponse("info-card", resp); err != nil {
			t.Errorf("ack response failed validation: %v", err)
		}
	})

	t.Run("response-cancellation-vocabulary", func(t *testing.T) {
		t.Helper()
		canonical := &envelopes.Response{
			V:          envelopes.ProtocolVersion,
			EnvelopeID: "envelopestest_cancel_canonical",
			Kind:       envelopes.ResponseKindAck,
			Status:     envelopes.ResponseStatusCanceled,
		}
		if err := reg.ValidateResponse("info-card", canonical); err != nil {
			t.Errorf("canonical canceled response failed validation: %v", err)
		}

		legacy := *canonical
		legacy.EnvelopeID = "envelopestest_cancel_legacy"
		// Model decoded v0.2 wire data directly; new source should not use the
		// deprecated British-spelled exported constant.
		legacy.Status = envelopes.ResponseStatus("cancelled")
		if err := reg.ValidateResponse("info-card", &legacy); err != nil {
			t.Errorf("legacy cancelled response failed v0.4.x compatibility validation: %v", err)
		}
		if got := legacy.Status.Canonical(); got != envelopes.ResponseStatusCanceled {
			t.Errorf("legacy status canonicalized to %q", got)
		}
	})
}
