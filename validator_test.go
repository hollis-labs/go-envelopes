package envelopes

import (
	"context"
	"errors"
	"testing"
)

func mustLoad(t *testing.T) *Registry {
	t.Helper()
	r, err := LoadCore(context.Background())
	if err != nil {
		t.Fatalf("LoadCore: %v", err)
	}
	return r
}

func TestValidateEnvelope_success(t *testing.T) {
	r := mustLoad(t)
	env := &Envelope{
		V:    1,
		ID:   "env_1",
		Type: "info-card",
		Data: map[string]any{
			"title": "Hello",
			"body":  "world",
		},
	}
	if err := r.ValidateEnvelope(env); err != nil {
		t.Fatalf("ValidateEnvelope: %v", err)
	}
}

func TestValidateEnvelope_schemaFailure(t *testing.T) {
	r := mustLoad(t)
	env := &Envelope{
		V:    1,
		ID:   "env_2",
		Type: "info-card",
		Data: map[string]any{
			// missing required "body"
			"title": "Hello",
		},
	}
	err := r.ValidateEnvelope(env)
	if err == nil {
		t.Fatal("expected schema validation failure")
	}
	if !errors.Is(err, ErrSchemaValidation) {
		t.Errorf("expected ErrSchemaValidation, got %v", err)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError in chain, got %T", err)
	} else if ve.Type != "info-card" {
		t.Errorf("ValidationError.Type = %q, want info-card", ve.Type)
	}
}

func TestValidateEnvelope_unknownType(t *testing.T) {
	r := mustLoad(t)
	env := &Envelope{V: 1, ID: "env_3", Type: "no-such-type"}
	err := r.ValidateEnvelope(env)
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("expected ErrUnknownType, got %v", err)
	}
}

func TestValidateEnvelope_typeWithoutSchema(t *testing.T) {
	r := mustLoad(t)
	// message-request is declared but ships no schema file — validation
	// should pass on type-name alone.
	env := &Envelope{
		V:    1,
		ID:   "env_4",
		Type: "message-request",
		Data: map[string]any{"anything": "goes"},
	}
	if err := r.ValidateEnvelope(env); err != nil {
		t.Errorf("expected nil for schema-less type, got %v", err)
	}
}

func TestValidateEnvelope_baseShape(t *testing.T) {
	r := mustLoad(t)
	tests := []struct {
		name string
		env  *Envelope
		want string
	}{
		{"nil", nil, "nil envelope"},
		{"wrong v", &Envelope{V: 2, ID: "id", Type: "info-card"}, "unsupported protocol version"},
		{"empty id", &Envelope{V: 1, Type: "info-card"}, "id is empty"},
		{"empty type", &Envelope{V: 1, ID: "id"}, "type is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := r.ValidateEnvelope(tt.env)
			if err == nil {
				t.Fatal("expected error")
			}
			if !contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidateResponse_canonicalKinds(t *testing.T) {
	r := mustLoad(t)
	good := []*Response{
		{V: 1, EnvelopeID: "env_1", Kind: ResponseKindData, Status: ResponseStatusSubmitted},
		{V: 1, EnvelopeID: "env_1", Kind: ResponseKindAck, Status: ResponseStatusSubmitted},
		{V: 1, EnvelopeID: "env_1", Kind: ResponseKindUI, Status: ResponseStatusSubmitted, Action: "dismissed"},
		{V: 1, EnvelopeID: "env_1", Kind: ResponseKindAsyncAck, Status: ResponseStatusSubmitted,
			Handle: &Handle{WorkflowInstanceID: "wf_1"}},
		{V: 1, EnvelopeID: "env_1", Kind: ResponseKindError, Status: ResponseStatusError,
			Error: &ResponseError{Code: ErrorCodeValidationFailed, Message: "bad"}},
	}
	for _, resp := range good {
		if err := r.ValidateResponse("info-card", resp); err != nil {
			t.Errorf("kind=%s: %v", resp.Kind, err)
		}
	}
}

func TestValidateResponse_errorMissingCode(t *testing.T) {
	r := mustLoad(t)
	resp := &Response{V: 1, EnvelopeID: "env_1", Kind: ResponseKindError, Status: ResponseStatusError}
	if err := r.ValidateResponse("info-card", resp); err == nil {
		t.Fatal("expected error response without code to fail")
	}
}

func TestValidateResponse_unknownKind(t *testing.T) {
	r := mustLoad(t)
	resp := &Response{V: 1, EnvelopeID: "env_1", Kind: "totally-bogus", Status: ResponseStatusSubmitted}
	err := r.ValidateResponse("info-card", resp)
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("expected ErrUnsupportedKind, got %v", err)
	}
}

func TestValidateResponse_unknownEnvelopeType(t *testing.T) {
	r := mustLoad(t)
	resp := &Response{V: 1, EnvelopeID: "env_1", Kind: ResponseKindAck, Status: ResponseStatusSubmitted}
	err := r.ValidateResponse("nope", resp)
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("expected ErrUnknownType, got %v", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
