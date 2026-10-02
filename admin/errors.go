package admin

// Failure carries the normalized command error without importing net/http.
// Its message and details MUST be sanitized by their author.
type Failure struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Errors  []ValidationError `json:"errors,omitempty"`
}

func (f *Failure) Error() string { return f.Code + ": " + f.Message }

type ErrorResponse struct {
	Error Failure `json:"error"`
}

const (
	MalformedInput       = "malformed_input"
	Unauthenticated      = "unauthenticated"
	Forbidden            = "forbidden"
	UnknownResource      = "unknown_resource"
	ManifestChanged      = "manifest_changed"
	ValueConflict        = "value_conflict"
	ValidationFailed     = "validation_failed"
	PreconditionRequired = "precondition_required"
	Unsupported          = "unsupported"
	BackendUnavailable   = "backend_unavailable"
)

// StatusCode maps contract errors; arbitrary/unknown host failures map to 503.
func (f *Failure) StatusCode() int {
	switch f.Code {
	case MalformedInput:
		return 400
	case Unauthenticated:
		return 401
	case Forbidden:
		return 403
	case UnknownResource:
		return 404
	case ManifestChanged:
		return 409
	case ValueConflict:
		return 412
	case ValidationFailed:
		return 422
	case PreconditionRequired:
		return 428
	case Unsupported:
		return 501
	default:
		return 503
	}
}
