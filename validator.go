package envelopes

import (
	"fmt"
)

// ValidateEnvelope checks env against its registered type spec.
//
// Returns:
//   - ErrUnknownType when env.Type is not registered.
//   - *ValidationError (which Is(ErrSchemaValidation)) when env.Data does
//     not match the registered DataSchema.
//   - nil on success, including when the type is registered without a
//     DataSchema (only the type name is verified — schema-less types are
//     trusted).
//
// The base envelope shape (V, ID, Type) is enforced too: V must equal the
// protocol version this package implements; ID and Type must be non-empty.
func (r *Registry) ValidateEnvelope(env *Envelope) error {
	if env == nil {
		return fmt.Errorf("envelopes: nil envelope")
	}
	if env.V != ProtocolVersion {
		return fmt.Errorf("envelopes: unsupported protocol version %d (expected %d)", env.V, ProtocolVersion)
	}
	if env.ID == "" {
		return fmt.Errorf("envelopes: envelope id is empty")
	}
	if env.Type == "" {
		return fmt.Errorf("envelopes: envelope type is empty")
	}
	spec, ok := r.Lookup(env.Type)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownType, env.Type)
	}
	if spec.DataSchema == nil {
		return nil
	}
	// jsonschema/v6 expects an interface-shaped value (map[string]any /
	// []any), which a JSON-decoded map already satisfies. Pass Data as-is.
	if err := spec.DataSchema.Validate(any(env.Data)); err != nil {
		return &ValidationError{Type: env.Type, Inner: err}
	}
	return nil
}

// ValidateResponse checks resp against the response-kind contract for the
// envelope type that produced it. The envelope type is required because
// per-type response shapes vary (e.g., a core "info-card" may emit only
// ack/ui responses while "question-form" returns data).
//
// Validation rules:
//   - resp.V must equal ProtocolVersion.
//   - resp.EnvelopeID must be non-empty (echoes envelope.id).
//   - resp.Kind must be one of the canonical kinds.
//   - resp.Status must be one of the canonical statuses.
//   - When resp.Kind == data and the type registers a PayloadSchema, the
//     payload is validated against it.
//   - When resp.Kind == error, resp.Error must be non-nil with a Code.
//
// Cross-validation against the registered ResponseKind is intentionally
// loose in v0.1: we accept any canonical kind even when the type's spec
// declares a different default ResponseKind. Tightening this is a follow-on
// once the protocol's per-type response constraints are codified.
func (r *Registry) ValidateResponse(envelopeType string, resp *Response) error {
	if resp == nil {
		return fmt.Errorf("envelopes: nil response")
	}
	if resp.V != ProtocolVersion {
		return fmt.Errorf("envelopes: unsupported protocol version %d (expected %d)", resp.V, ProtocolVersion)
	}
	if resp.EnvelopeID == "" {
		return fmt.Errorf("envelopes: response envelopeId is empty")
	}
	if !resp.Kind.IsValid() {
		return fmt.Errorf("%w: %q", ErrUnsupportedKind, resp.Kind)
	}
	if !resp.Status.IsValid() {
		return fmt.Errorf("envelopes: invalid response status %q", resp.Status)
	}
	if envelopeType == "" {
		return fmt.Errorf("envelopes: envelope type is empty")
	}
	spec, ok := r.Lookup(envelopeType)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownType, envelopeType)
	}

	switch resp.Kind {
	case ResponseKindData:
		if spec.PayloadSchema != nil {
			if err := spec.PayloadSchema.Validate(any(resp.Payload)); err != nil {
				return &ValidationError{Type: envelopeType, Inner: err}
			}
		}
	case ResponseKindError:
		if resp.Error == nil || resp.Error.Code == "" {
			return fmt.Errorf("envelopes: error response missing error.code")
		}
	case ResponseKindAck, ResponseKindUI, ResponseKindAsyncAck:
		// No payload schema required by v1; type-specific checks are
		// up to the consumer.
	}
	return nil
}
