// Package admin owns admin contract v1 independently of the envelope catalog.
// It provides no storage, authentication, transport, listener or lifecycle action.
package admin

import "context"

// ContractVersion is independent of the module version and Envelope.V.
const ContractVersion = 1

type App struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Scope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type Endpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
type Capabilities struct {
	CanRead     bool `json:"can_read"`
	CanValidate bool `json:"can_validate"`
	CanUpdate   bool `json:"can_update"`
	CanReset    bool `json:"can_reset"`
}
type SourceKind string

const (
	OverrideSource SourceKind = "override"
	EnvSource      SourceKind = "env"
	FileSource     SourceKind = "file"
	DefaultSource  SourceKind = "default"
)

type Source struct {
	Kind  SourceKind `json:"kind"`
	Label string     `json:"label"`
}
type Resolution struct {
	Precedence []SourceKind `json:"precedence"`
	WriteLayer SourceKind   `json:"write_layer,omitempty"`
}
type ApplyState string

const (
	Active         ApplyState = "active"
	PendingRestart ApplyState = "pending_restart"
	UnknownApply   ApplyState = "unknown"
)

type ValidationError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Validation struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors"`
}

// Changes is intent. Both containers are required; nil means malformed, not empty.
type Changes struct {
	Set   map[string]Scalar `json:"set"`
	Unset []string          `json:"unset"`
}
type Command struct {
	Revision string `json:"revision"`
	Changes
}
type ResetCommand struct {
	Revision string   `json:"revision"`
	Keys     []string `json:"keys"`
}

// ResolvedValue is private candidate data, including kept secrets. It MUST NOT be
// logged. Direct JSON marshaling is rejected; use Group.Project for public reads.
type ResolvedValue struct {
	Present        bool
	Value          Scalar
	Source         *Source
	Editable       bool
	ReadOnlyReason string
	HasOverride    bool
	ApplyState     ApplyState
}

// State is a consistent host view. Version is an opaque generation, never a hash
// of secrets; all value, source, permission and apply changes invalidate it.
type State struct {
	Revision string
	Version  string
	Values   map[string]ResolvedValue
}

// ValueRecord is a redacted wire projection, never the privately resolved value.
type ValueRecord struct {
	Present        bool       `json:"present"`
	Value          *Scalar    `json:"value,omitempty"`
	Source         *Source    `json:"source,omitempty"`
	Editable       bool       `json:"editable"`
	ReadOnlyReason string     `json:"read_only_reason,omitempty"`
	HasOverride    bool       `json:"has_override"`
	SecretPresent  *bool      `json:"secret_present,omitempty"`
	ApplyState     ApplyState `json:"apply_state"`
}
type Snapshot struct {
	GroupID    string                 `json:"group_id"`
	Revision   string                 `json:"revision"`
	Values     map[string]ValueRecord `json:"values"`
	Validation Validation             `json:"validation"`
}

// Restart describes THIS command's changed desired values, not outstanding work.
// Pending work from previous commands remains in Snapshot.Values.ApplyState.
type Restart struct {
	Required bool
	Targets  []string
}
type UpdateResponse struct {
	Snapshot        Snapshot `json:"snapshot"`
	ChangedKeys     []string `json:"changed_keys"`
	RestartRequired bool     `json:"restart_required"`
	ApplyTargets    []string `json:"apply_targets"`
}

// GroupBackend must supply consistent reads and side-effect-free previews. The
// host owns resolution/fallbacks and all writers, including env/file/policy changes.
// WithTransaction MUST roll back on callback error and commit only on success;
// it must cover all stores and opaque version changes atomically. No lib mutex
// can substitute for this guarantee. Do not advertise mutations without it.
type GroupBackend interface {
	Read(context.Context) (State, error)
	Preview(context.Context, Changes) (State, error)
	WithTransaction(context.Context, func(GroupTransaction) error) error
}

// GroupTransaction runs under the host's transaction/CAS boundary. Current and
// Resolve include the authoritative revision and effective permissions. Stage
// prepares a post-write view WITHOUT committing; the enclosing callback's success
// commits it, failure rolls back. Stage must not invoke restart/live application.
// Restart metadata is supplied by the host and validated before commit.
type GroupTransaction interface {
	Current() (State, error)
	Resolve(Changes) (State, error)
	Stage(Changes) (Staged, error)
}

// SemanticValidator checks the complete resolved candidate without side effects.
// Messages are trusted sanitized text; never include secrets or raw errors.
type SemanticValidator func(context.Context, map[string]ResolvedValue) (Validation, error)

// Staged is a proposed post-commit view. Validate it with Group.UpdateResult
// before returning success from the transaction callback.
type Staged struct {
	State       State
	ChangedKeys []string
	Restart     Restart
}
