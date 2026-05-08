package envelopes

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by the registry, validator, and extension API.
// Use errors.Is to branch on these from caller code.
var (
	// ErrUnknownType is returned by Lookup, ValidateEnvelope, ValidateResponse,
	// and UnregisterType when the named envelope type is not registered.
	ErrUnknownType = errors.New("envelopes: unknown type")

	// ErrSchemaValidation is returned by ValidateEnvelope and ValidateResponse
	// when the payload does not match the registered JSON Schema. Wrap a
	// concrete *jsonschema.ValidationError; use errors.As to inspect details.
	ErrSchemaValidation = errors.New("envelopes: schema validation failed")

	// ErrConflict is returned by RegisterType when the name is already
	// registered. The library does not silently override; callers decide.
	ErrConflict = errors.New("envelopes: type already registered")

	// ErrInvalidName is returned by RegisterType when the type name does not
	// satisfy namespace requirements (plugin types must be "<id>.<name>";
	// core names are reserved by core registration).
	ErrInvalidName = errors.New("envelopes: invalid type name")

	// ErrCoreTypeProtected is returned by UnregisterType when the caller
	// attempts to remove a type whose Source is TypeSourceCore.
	ErrCoreTypeProtected = errors.New("envelopes: core types cannot be unregistered")

	// ErrUnsupportedKind is returned by ValidateResponse when the response
	// kind is unknown.
	ErrUnsupportedKind = errors.New("envelopes: unsupported response kind")
)

// ValidationError wraps a JSON Schema validation failure with the envelope
// type name that triggered it. Errors.Is(err, ErrSchemaValidation) returns
// true; errors.As(err, &ve) where ve is *jsonschema.ValidationError yields
// the underlying validator detail.
type ValidationError struct {
	Type  string
	Inner error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("envelopes: validate %q: %v", e.Type, e.Inner)
}

func (e *ValidationError) Unwrap() error { return e.Inner }

func (e *ValidationError) Is(target error) bool {
	return target == ErrSchemaValidation
}
