package envelopes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
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
	Type     string
	Inner    error
	Failures []ValidationFailure
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("envelopes: validate %q: %v", e.Type, e.Inner)
}

func (e *ValidationError) Unwrap() error { return e.Inner }

func (e *ValidationError) Is(target error) bool {
	return target == ErrSchemaValidation
}

// ValidationFailure is one actionable leaf in a JSON Schema validation error.
// Paths are RFC 6901 JSON pointers. Expected values come from the trusted
// schema. Actual deliberately uses a bounded summary rather than echoing
// arbitrary payload content into logs or prompts.
type ValidationFailure struct {
	InstancePath string          `json:"instancePath"`
	SchemaPath   string          `json:"schemaPath"`
	SchemaURI    string          `json:"schemaURI,omitempty"`
	Keyword      string          `json:"keyword,omitempty"`
	Message      string          `json:"message"`
	Expected     any             `json:"expected,omitempty"`
	Actual       *ValueSummary   `json:"actual,omitempty"`
	Metadata     *SchemaMetadata `json:"metadata,omitempty"`
}

// ValueSummary describes an invalid value without copying unbounded or secret
// string/object content. Value is populated only for booleans, JSON numbers,
// and validation counts; string, array, and object values expose length only.
type ValueSummary struct {
	Type   string `json:"type"`
	Value  any    `json:"value,omitempty"`
	Length *int   `json:"length,omitempty"`
}

// Details returns a defensive copy of the structured validation failures.
func (e *ValidationError) Details() []ValidationFailure {
	if e == nil {
		return nil
	}
	out := make([]ValidationFailure, len(e.Failures))
	for i, failure := range e.Failures {
		out[i] = cloneValidationFailure(failure)
	}
	return out
}

func newValidationError(typeName string, inner error, document *SchemaDocument) *ValidationError {
	return &ValidationError{
		Type:     typeName,
		Inner:    inner,
		Failures: FlattenValidationFailures(inner, document),
	}
}

// FlattenValidationFailures converts a jsonschema.ValidationError tree into a
// deterministic list of actionable leaf failures. It returns nil for errors
// that are not JSON Schema validation errors.
func FlattenValidationFailures(err error, document *SchemaDocument) []ValidationFailure {
	var validationError *jsonschema.ValidationError
	if !errors.As(err, &validationError) {
		return nil
	}
	failures := make([]ValidationFailure, 0)
	collectValidationFailures(validationError, document, &failures)
	return failures
}

func collectValidationFailures(validationError *jsonschema.ValidationError, document *SchemaDocument, failures *[]ValidationFailure) {
	if validationError == nil {
		return
	}
	if len(validationError.Causes) > 0 {
		for _, cause := range validationError.Causes {
			collectValidationFailures(cause, document, failures)
		}
		return
	}
	*failures = append(*failures, validationFailureFromLeaf(validationError, document))
}

func validationFailureFromLeaf(validationError *jsonschema.ValidationError, document *SchemaDocument) ValidationFailure {
	keywordPath := validationError.ErrorKind.KeywordPath()
	failure := ValidationFailure{
		InstancePath: jsonPointer(validationError.InstanceLocation),
		SchemaPath:   schemaPathFromLocation(validationError.SchemaURL, keywordPath),
		SchemaURI:    schemaURI(validationError.SchemaURL),
		Message:      validationErrorMessage(validationError),
	}
	if len(keywordPath) > 0 {
		failure.Keyword = keywordPath[0]
	}
	if document != nil {
		nodePath := schemaPathFromLocation(validationError.SchemaURL, nil)
		if metadata, ok := document.MetadataAtSchemaPath(nodePath); ok {
			failure.Metadata = &metadata
		}
	}
	failure.Expected, failure.Actual = expectedActual(validationError.ErrorKind, failure.Metadata)
	return failure
}

func validationErrorMessage(validationError *jsonschema.ValidationError) string {
	switch validationError.ErrorKind.(type) {
	case *kind.Pattern:
		return "value does not match required pattern"
	case *kind.Format:
		return "value does not match required format"
	case *kind.AdditionalProperties:
		return "additional properties are not allowed"
	case *kind.PropertyNames:
		return "property name is invalid"
	}
	output := validationError.BasicOutput()
	if output != nil && output.Error != nil {
		return output.Error.String()
	}
	return validationError.Error()
}

func expectedActual(errorKind jsonschema.ErrorKind, metadata *SchemaMetadata) (any, *ValueSummary) {
	switch current := errorKind.(type) {
	case *kind.Type:
		return append([]string(nil), current.Want...), &ValueSummary{Type: current.Got}
	case *kind.Enum:
		return cloneJSONValue(current.Want), summarizeValue(current.Got)
	case *kind.Const:
		return cloneJSONValue(current.Want), summarizeValue(current.Got)
	case *kind.Format:
		return current.Want, summarizeValue(current.Got)
	case *kind.Required:
		return append([]string(nil), current.Missing...), nil
	case *kind.Dependency:
		return append([]string(nil), current.Missing...), nil
	case *kind.DependentRequired:
		return append([]string(nil), current.Missing...), nil
	case *kind.AdditionalProperties:
		expected := any(nil)
		if metadata != nil {
			expected = append([]string(nil), metadata.Properties...)
		}
		return expected, summarizePropertyNames(current.Properties)
	case *kind.PropertyNames:
		return nil, summarizeValue(current.Property)
	case *kind.MinProperties:
		return current.Want, countSummary("object", current.Got)
	case *kind.MaxProperties:
		return current.Want, countSummary("object", current.Got)
	case *kind.MinItems:
		return current.Want, countSummary("array", current.Got)
	case *kind.MaxItems:
		return current.Want, countSummary("array", current.Got)
	case *kind.MinLength:
		return current.Want, countSummary("string", current.Got)
	case *kind.MaxLength:
		return current.Want, countSummary("string", current.Got)
	case *kind.Pattern:
		return current.Want, summarizeValue(current.Got)
	case *kind.Minimum:
		return current.Want.RatString(), &ValueSummary{Type: "number", Value: current.Got.RatString()}
	case *kind.Maximum:
		return current.Want.RatString(), &ValueSummary{Type: "number", Value: current.Got.RatString()}
	case *kind.ExclusiveMinimum:
		return current.Want.RatString(), &ValueSummary{Type: "number", Value: current.Got.RatString()}
	case *kind.ExclusiveMaximum:
		return current.Want.RatString(), &ValueSummary{Type: "number", Value: current.Got.RatString()}
	case *kind.MultipleOf:
		return current.Want.RatString(), &ValueSummary{Type: "number", Value: current.Got.RatString()}
	case *kind.UniqueItems:
		return "unique items", countSummary("duplicate indexes", 2)
	default:
		return nil, nil
	}
}

func summarizePropertyNames(properties []string) *ValueSummary {
	length := len(properties)
	return &ValueSummary{Type: "property-names", Length: &length}
}

func countSummary(valueType string, count int) *ValueSummary {
	return &ValueSummary{Type: valueType, Value: count}
}

func summarizeValue(value any) *ValueSummary {
	switch value := value.(type) {
	case nil:
		return &ValueSummary{Type: "null"}
	case string:
		length := len(value)
		return &ValueSummary{Type: "string", Length: &length}
	case bool:
		return &ValueSummary{Type: "boolean", Value: value}
	case json.Number:
		return &ValueSummary{Type: "number", Value: value.String()}
	case float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return &ValueSummary{Type: "number", Value: value}
	case []any:
		length := len(value)
		return &ValueSummary{Type: "array", Length: &length}
	case map[string]any:
		length := len(value)
		return &ValueSummary{Type: "object", Length: &length}
	default:
		valueType := reflect.TypeOf(value)
		if valueType == nil {
			return &ValueSummary{Type: "null"}
		}
		return &ValueSummary{Type: valueType.String()}
	}
}

func schemaURI(location string) string {
	parsed, err := url.Parse(location)
	if err == nil {
		parsed.Fragment = ""
		return parsed.String()
	}
	if index := strings.IndexByte(location, '#'); index >= 0 {
		return location[:index]
	}
	return location
}

func cloneValidationFailure(failure ValidationFailure) ValidationFailure {
	failure.Expected = cloneJSONValue(failure.Expected)
	if failure.Actual != nil {
		actual := *failure.Actual
		actual.Value = cloneJSONValue(actual.Value)
		if actual.Length != nil {
			length := *actual.Length
			actual.Length = &length
		}
		failure.Actual = &actual
	}
	if failure.Metadata != nil {
		metadata := *failure.Metadata
		metadata.Default = cloneJSONValue(metadata.Default)
		metadata.Examples = anySlice(metadata.Examples)
		metadata.Types = append([]string(nil), metadata.Types...)
		metadata.Enum = anySlice(metadata.Enum)
		metadata.Required = append([]string(nil), metadata.Required...)
		metadata.Properties = append([]string(nil), metadata.Properties...)
		if metadata.AdditionalPropertiesAllowed != nil {
			allowed := *metadata.AdditionalPropertiesAllowed
			metadata.AdditionalPropertiesAllowed = &allowed
		}
		metadata.Custom = cloneStringAnyMap(metadata.Custom)
		failure.Metadata = &metadata
	}
	return failure
}
