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
//
// Answers and Decisions are typed return channels that sit alongside Payload
// rather than inside it. An interactive envelope returns up to three
// different things — freeform data, answers to questions, and decisions on
// items — and collapsing them into one untyped blob loses the distinction at
// exactly the point a consumer needs it. Both are optional; a card that
// returns only freeform data uses Payload alone, as before.
type Response struct {
	V           int            `json:"v"`
	EnvelopeID  string         `json:"envelopeId"`
	Kind        ResponseKind   `json:"kind"`
	Status      ResponseStatus `json:"status"`
	Payload     any            `json:"payload,omitempty"`
	Answers     []Answer       `json:"answers,omitempty"`
	Decisions   []Decision     `json:"decisions,omitempty"`
	Handle      *Handle        `json:"handle,omitempty"`
	Action      string         `json:"action,omitempty"`
	Error       *ResponseError `json:"error,omitempty"`
	CompletedAt string         `json:"completedAt,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// Answer is one reply to one question posed by an envelope.
//
// AcceptedSuggestion is a pointer so that "the user explicitly rejected the
// suggestion" is distinguishable from "this envelope offered no suggestion" —
// a bare false cannot carry that difference.
type Answer struct {
	QuestionID         string `json:"questionId"`
	Value              any    `json:"value"`
	AcceptedSuggestion *bool  `json:"acceptedSuggestion,omitempty"`
	Note               string `json:"note,omitempty"`
}

// Decision is one disposition of one item presented by an envelope.
//
// Action is deliberately an open string: the set of dispositions belongs to
// the interaction, not to the wire format. A triage card's "defer" and an
// approval queue's "escalate" are both valid, and enumerating them here would
// make the protocol the bottleneck for every new interaction.
type Decision struct {
	ItemID string         `json:"itemId"`
	Action string         `json:"action"`
	Note   string         `json:"note,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
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
	ResponseStatusCanceled  ResponseStatus = "canceled"
	ResponseStatusPartial   ResponseStatus = "partial"
	ResponseStatusError     ResponseStatus = "error"

	// ResponseStatusCancelled is the legacy British-spelled wire value.
	//
	// Deprecated: use ResponseStatusCanceled for new responses. The legacy
	// spelling remains recognized throughout v0.4.x so v0.2-era persisted
	// responses can be read and existing emitters can migrate without a flag
	// day. It is scheduled for removal in v0.5.0.
	ResponseStatusCancelled ResponseStatus = "cancelled"
)

// IsCanonical reports whether s is one of the canonical response statuses.
// The legacy ResponseStatusCancelled compatibility value is not canonical.
func (s ResponseStatus) IsCanonical() bool {
	switch s {
	case ResponseStatusSubmitted, ResponseStatusCanceled,
		ResponseStatusPartial, ResponseStatusError:
		return true
	}
	return false
}

// IsValid reports whether s is a recognized response status. In v0.4.x this
// includes the legacy ResponseStatusCancelled wire value; new responses should
// use a status for which IsCanonical reports true.
func (s ResponseStatus) IsValid() bool {
	return s.IsCanonical() || s == ResponseStatusCancelled
}

// Canonical returns the canonical US-English spelling of s. It maps the v0.4.x
// legacy cancellation value to ResponseStatusCanceled and leaves all other
// values unchanged. Call IsValid before Canonical when rejecting unknown input.
func (s ResponseStatus) Canonical() ResponseStatus {
	if s == ResponseStatusCancelled {
		return ResponseStatusCanceled
	}
	return s
}

// IsTerminal reports whether s closes the interaction.
//
// This is the distinction a host needs to decide what a SECOND submission
// against the same envelope means, and it is the one piece of response
// semantics that was previously left for every host to reinvent:
//
//   - Terminal (submitted, canceled, error): the interaction is resolved. The
//     host should record the response immutably and answer a later submission
//     with a conflict carrying the response it already has, rather than a bare
//     error — the caller usually wants to reflect the resolved state, not
//     retry.
//   - Non-terminal (partial): the interaction is still in progress. A partial
//     response is a resumable draft and a later submission REPLACES it. A host
//     that claims the envelope on a partial submission makes its own protocol
//     unreachable: the interaction can never be completed, because the
//     completing submission collides with the draft that preceded it.
//
// The legacy "cancelled" spelling is terminal, like the canonical spelling it
// maps to. An unrecognized status is not terminal — an unknown state is not a
// resolution, and treating it as one would discard a response.
func (s ResponseStatus) IsTerminal() bool {
	switch s.Canonical() {
	case ResponseStatusSubmitted, ResponseStatusCanceled, ResponseStatusError:
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
	ErrorCodeUserCanceled        = "user-canceled"

	// ErrorCodeUserCancelled is the legacy British-spelled wire value.
	//
	// Deprecated: use ErrorCodeUserCanceled for new responses. The legacy value
	// remains readable throughout v0.4.x and is scheduled for removal in v0.5.0.
	ErrorCodeUserCancelled = "user-cancelled"
)

// CanonicalErrorCode returns the canonical US-English spelling for a known
// legacy error code and preserves all other codes, including extension codes.
func CanonicalErrorCode(code string) string {
	if code == ErrorCodeUserCancelled {
		return ErrorCodeUserCanceled
	}
	return code
}

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
// DataSchemaDocument and PayloadSchemaDocument retain module-owned source JSON
// and parsed metadata alongside their compiled validators. When a document is
// present RegisterType compiles it and replaces the corresponding compiled
// schema, making the exportable source authoritative. TypeScript carries typed
// generator/import metadata. UIMetadata remains as a compatibility view of the
// same import hints for v0.3 consumers.
type TypeSpec struct {
	Name                  string
	Version               string
	DataSchema            *jsonschema.Schema
	DataSchemaDocument    *SchemaDocument
	ResponseKind          ResponseKind
	PayloadSchema         *jsonschema.Schema
	PayloadSchemaDocument *SchemaDocument
	Description           string
	Source                TypeSource
	PluginID              string
	TypeScript            TypeScriptMetadata

	// UIMetadata is retained for source compatibility with v0.3 consumers, but
	// RegisterType normalizes its values to canonical JSON Go shapes. New
	// generators should use TypeScript.Import, whose typed fields avoid repeating
	// string-key lookups at every consumer.
	UIMetadata map[string]any
}

// TypeScriptMetadata is module-owned metadata used by TypeScript generators.
// DataType is the stable exported data-type identifier derived from Name.
type TypeScriptMetadata struct {
	DataType string         `json:"dataType"`
	Import   ImportMetadata `json:"import,omitempty"`
}

// ImportMetadata describes a host component import without prescribing how a
// particular host loads or renders that component.
type ImportMetadata struct {
	Component string         `json:"component,omitempty"`
	Export    string         `json:"export,omitempty"`
	Props     string         `json:"props,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}
