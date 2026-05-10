package envelopes

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ProtocolVersion is the wire-protocol version this package implements.
const ProtocolVersion = 1

// Envelope is the typed wire format an agent sends to a host.
//
// The Data field carries the type-specific payload as raw JSON-shaped values
// (typically a map[string]any). Per-type Go structs are NOT generated in
// this package; consumers that want typed access can deserialize Data into
// their own struct after Validate succeeds.
type Envelope struct {
	V            int            `json:"v"`
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	TypeVersion  string         `json:"typeVersion,omitempty"`
	Title        string         `json:"title,omitempty"`
	Context      string         `json:"context,omitempty"`
	Presentation Presentation   `json:"presentation,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
	Trace        *Trace         `json:"trace,omitempty"`
	Meta         map[string]any `json:"meta,omitempty"`
}

// Trace carries observability metadata.
type Trace struct {
	AgentID          string `json:"agentId,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	ParentEnvelopeID string `json:"parentEnvelopeId,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
}

// Presentation hints at preferred rendering. Hosts MAY honor, MAY substitute,
// MUST NOT fail on an unsupported presentation.
type Presentation string

const (
	PresentationInline     Presentation = "inline"
	PresentationModal      Presentation = "modal"
	PresentationDrawer     Presentation = "drawer"
	PresentationSidecar    Presentation = "sidecar"
	PresentationFullscreen Presentation = "fullscreen"
)

// Response is the structured reply a host returns for an envelope.
//
// The Kind field discriminates which optional payload-bearing field carries
// data: Payload (Kind == data), Handle (Kind == ack | async-ack), Action
// (Kind == ui), or Error (Kind == error).
type Response struct {
	V           int            `json:"v"`
	EnvelopeID  string         `json:"envelopeId"`
	Kind        ResponseKind   `json:"kind"`
	Status      ResponseStatus `json:"status"`
	Payload     any            `json:"payload,omitempty"`
	Handle      *Handle        `json:"handle,omitempty"`
	Action      string         `json:"action,omitempty"`
	Error       *ResponseError `json:"error,omitempty"`
	CompletedAt string         `json:"completedAt,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// ResponseKind discriminates payload semantics.
type ResponseKind string

const (
	ResponseKindData     ResponseKind = "data"
	ResponseKindAck      ResponseKind = "ack"
	ResponseKindUI       ResponseKind = "ui"
	ResponseKindAsyncAck ResponseKind = "async-ack"
	ResponseKindError    ResponseKind = "error"
)

// IsValid reports whether k is one of the canonical response kinds.
func (k ResponseKind) IsValid() bool {
	switch k {
	case ResponseKindData, ResponseKindAck, ResponseKindUI,
		ResponseKindAsyncAck, ResponseKindError:
		return true
	}
	return false
}

// ResponseStatus describes the outcome of the user's interaction with the
// envelope.
type ResponseStatus string

const (
	ResponseStatusSubmitted ResponseStatus = "submitted"
	ResponseStatusCancelled ResponseStatus = "cancelled"
	ResponseStatusPartial   ResponseStatus = "partial"
	ResponseStatusError     ResponseStatus = "error"
)

// IsValid reports whether s is one of the canonical response statuses.
func (s ResponseStatus) IsValid() bool {
	switch s {
	case ResponseStatusSubmitted, ResponseStatusCancelled,
		ResponseStatusPartial, ResponseStatusError:
		return true
	}
	return false
}

// Handle is the optional handle returned with ack and async-ack responses.
// Fields are populated based on Kind: ack uses URI/ResourceID/MIME;
// async-ack uses WorkflowInstanceID/SubscribeTo.
type Handle struct {
	URI                string `json:"uri,omitempty"`
	ResourceID         string `json:"resourceId,omitempty"`
	MIME               string `json:"mime,omitempty"`
	WorkflowInstanceID string `json:"workflowInstanceId,omitempty"`
	SubscribeTo        string `json:"subscribeTo,omitempty"`
}

// ResponseError describes a protocol-level error response.
type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

// Canonical error codes used by the Envelope UI Protocol's response
// envelope (kind == error). Hosts and agents SHOULD prefer these codes
// for cross-host interoperability; the set is intentionally small and
// extension-free.
const (
	ErrorCodeValidationFailed    = "validation-failed"
	ErrorCodeUnsupportedType     = "unsupported-type"
	ErrorCodeUnsupportedVersion  = "unsupported-version"
	ErrorCodeCapabilityDenied    = "capability-denied"
	ErrorCodeComponentLoadFailed = "component-load-failed"
	ErrorCodeTimeout             = "timeout"
	ErrorCodeHostError           = "host-error"
	ErrorCodeUserCancelled       = "user-cancelled"
)

// TypeSource identifies whether a registered type was loaded from the core
// manifest or registered at runtime by a plugin.
type TypeSource int

const (
	TypeSourceCore TypeSource = iota
	TypeSourcePlugin
)

// String returns a stable label suitable for logs and error messages.
func (s TypeSource) String() string {
	switch s {
	case TypeSourceCore:
		return "core"
	case TypeSourcePlugin:
		return "plugin"
	default:
		return "unknown"
	}
}

// TypeSpec describes one envelope type registered in a Registry.
//
// DataSchema is nil if the manifest entry did not ship a per-type JSON
// Schema (e.g. message-* types in the core seed). PayloadSchema is
// populated only when ResponseKind == data and the type ships a schema.
//
// UIMetadata carries TS-side rendering hints (component, export, props)
// extracted verbatim from the YAML manifest. Go consumers ignore these
// fields; downstream tooling (TS codegen, plugin scaffolders) reads them
// through Registry.All().
type TypeSpec struct {
	Name          string
	Version       string
	DataSchema    *jsonschema.Schema
	ResponseKind  ResponseKind
	PayloadSchema *jsonschema.Schema
	Description   string
	Source        TypeSource
	PluginID      string
	UIMetadata    map[string]any
}
