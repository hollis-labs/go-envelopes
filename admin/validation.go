package admin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

func validSource(s SourceKind) bool {
	return s == OverrideSource || s == EnvSource || s == FileSource || s == DefaultSource
}
func validApply(s ApplyState) bool { return s == Active || s == PendingRestart || s == UnknownApply }
func (g Group) validProfile() bool {
	keys := map[string]bool{}
	editable := false
	for _, f := range g.Fields {
		if !validID(f.Key) || keys[f.Key] {
			return false
		}
		keys[f.Key] = true
		if f.Type != StringType && f.Type != BooleanType && f.Type != IntegerType && f.Type != NumberType {
			return false
		}
		editable = editable || f.Editable
		if (!f.Editable && !nonempty(f.ReadOnlyReason)) || (f.RestartRequired && !validID(f.ApplyTarget)) || (!f.RestartRequired && f.ApplyTarget != "") {
			return false
		}
		if f.Secret && (f.Type != StringType || f.Default != nil || f.Enum != nil) {
			return false
		}
		if f.Default != nil && !f.Default.valid(f.Type) {
			return false
		}
		if f.Enum != nil && len(f.Enum) == 0 {
			return false
		}
		enum := map[any]bool{}
		for _, v := range f.Enum {
			if !v.valid(f.Type) || enum[v.value] {
				return false
			}
			enum[v.value] = true
		}
		for _, n := range []*float64{f.Minimum, f.Maximum} {
			if n != nil && ((f.Type != IntegerType && f.Type != NumberType) || math.IsNaN(*n) || math.IsInf(*n, 0)) {
				return false
			}
		}
		for _, n := range []*int{f.MinLength, f.MaxLength} {
			if n != nil && (f.Type != StringType || *n < 0 || uint64(*n) > MaxSafeInteger) {
				return false
			}
		}
		if (f.Minimum != nil && f.Maximum != nil && *f.Minimum > *f.Maximum) || (f.MinLength != nil && f.MaxLength != nil && *f.MinLength > *f.MaxLength) {
			return false
		}
		if f.Pattern != "" {
			if f.Type != StringType {
				return false
			}
			if _, err := portablePattern(f.Pattern); err != nil {
				return false
			}
		}
	}
	return editable || (!g.Capabilities.CanUpdate && !g.Capabilities.CanReset)
}
func (v ResolvedValue) MarshalJSON() ([]byte, error) {
	return nil, errors.New("admin: private resolved value; project before serialization")
}
func (s State) MarshalJSON() ([]byte, error) {
	return nil, errors.New("admin: private state; project before serialization")
}

// ETag quotes a host-issued opaque token. It does not derive a token from values.
func (s State) ETag() (string, error) {
	if s.Version == "" {
		return "", errors.New("admin: missing version")
	}
	for _, c := range s.Version {
		if c < 33 || c > 126 || c == '"' {
			return "", errors.New("admin: invalid version")
		}
	}
	return `"` + s.Version + `"`, nil
}
func (g Group) stateShape(s State) error {
	if !g.validProfile() || !nonempty(s.Revision) || len(s.Values) != len(g.Fields) {
		return errors.New("admin: invalid resolved state")
	}
	if _, err := s.ETag(); err != nil {
		return err
	}
	for _, f := range g.Fields {
		v, ok := s.Values[f.Key]
		if !ok || (!f.Editable && v.Editable) || (!v.Editable && !nonempty(v.ReadOnlyReason)) || !validApply(v.ApplyState) {
			return errors.New("admin: conflicting state metadata")
		}
		if v.Present {
			if !v.Value.valid(f.Type) || v.Source == nil || !validSource(v.Source.Kind) || !nonempty(v.Source.Label) {
				return errors.New("admin: invalid resolved value")
			}
			enabled := false
			for _, layer := range g.Resolution.Precedence {
				enabled = enabled || layer == v.Source.Kind
			}
			if !enabled {
				return errors.New("admin: undeclared source layer")
			}
		} else if v.Source != nil || v.Value.value != nil {
			return errors.New("admin: absent values must omit source and value")
		}
	}
	return nil
}

// Validate checks complete desired configuration, not a patch; existing private
// secrets participate. No defaults are inserted and no backend callbacks run.
// Shape errors are adapter failures, distinct from an invalid desired config.
func (g Group) Validate(s State) (Validation, error) {
	if err := g.stateShape(s); err != nil {
		return Validation{}, err
	}
	result := Validation{Valid: true, Errors: []ValidationError{}}
	add := func(key, code, message string) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{Path: pointer(key), Code: code, Message: message})
	}
	for _, f := range g.Fields {
		v := s.Values[f.Key]
		if !v.Present {
			if f.Required {
				add(f.Key, "required", "A value is required.")
			}
			continue
		}
		value := v.Value.value
		if f.Enum != nil {
			found := false
			for _, option := range f.Enum {
				found = found || option.value == value
			}
			if !found {
				add(f.Key, "enum", "Choose a declared value.")
			}
		}
		if n, ok := value.(float64); ok && ((f.Minimum != nil && n < *f.Minimum) || (f.Maximum != nil && n > *f.Maximum)) {
			add(f.Key, "range", "Number is outside the allowed range.")
		}
		if text, ok := value.(string); ok {
			length := utf8.RuneCountInString(text)
			if (f.MinLength != nil && length < *f.MinLength) || (f.MaxLength != nil && length > *f.MaxLength) {
				add(f.Key, "length", "Text length is outside the allowed range.")
			}
			if f.Pattern != "" {
				re, _ := portablePattern(f.Pattern)
				if !re.MatchString(text) {
					add(f.Key, "pattern", "Text does not match the declared pattern.")
				}
			}
		}
	}
	return result, nil
}
func pointer(key string) string {
	return "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

// Project validates adapter metadata and computes structural/schema validation,
// then emits only public values. Invalid desired constraints remain representable
// for recovery; malformed adapter types/permissions fail closed.
func (g Group) Project(ctx context.Context, s State) (Snapshot, error) {
	validation, err := g.ValidateCandidate(ctx, s)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{GroupID: g.ID, Revision: s.Revision, Values: map[string]ValueRecord{}, Validation: validation}
	for _, f := range g.Fields {
		v := s.Values[f.Key]
		record := ValueRecord{Present: v.Present, Editable: v.Editable, ReadOnlyReason: v.ReadOnlyReason, HasOverride: v.HasOverride, ApplyState: v.ApplyState}
		if v.Present {
			record.Source = copyPtr(v.Source)
		}
		if f.Secret {
			record.SecretPresent = copyPtr(&v.Present)
		} else if v.Present {
			record.Value = copyPtr(&v.Value)
		}
		snapshot.Values[f.Key] = record
	}
	return snapshot, nil
}

// CheckChanges checks the command shape and CURRENT effective permissions before
// host resolution. The resolved COMPLETE candidate still needs Validate and the
// host semantic validator. Empty strings are ordinary replacements, never keep.
func (g Group) CheckChanges(s State, c Changes) *Failure {
	fail := func(code string) *Failure {
		return &Failure{Code: code, Message: "The settings command cannot be accepted."}
	}
	if err := g.stateShape(s); err != nil {
		return fail(BackendUnavailable)
	}
	if c.Set == nil || c.Unset == nil {
		return fail(MalformedInput)
	}
	fields := map[string]Field{}
	for _, f := range g.Fields {
		fields[f.Key] = f
	}
	seen := map[string]bool{}
	for key, value := range c.Set {
		f, ok := fields[key]
		if !ok || !value.valid(f.Type) {
			return fail(MalformedInput)
		}
		if !g.Capabilities.CanUpdate || !f.Editable || !s.Values[key].Editable {
			return fail(Forbidden)
		}
		seen[key] = true
	}
	for _, key := range c.Unset {
		f, ok := fields[key]
		if !ok || seen[key] {
			return fail(MalformedInput)
		}
		seen[key] = true
		if !g.Capabilities.CanReset || !f.Editable || !s.Values[key].Editable {
			return fail(Forbidden)
		}
	}
	return nil
}

// UpdateResult validates the staged adapter answer BEFORE the enclosing host
// transaction commits. ChangedKeys is supplied by the host, in declaration order;
// restart targets are host-supplied, never derived/guessed here. A no-op must keep
// existing pending apply state while reporting no command restart work.
func (g Group) UpdateResult(ctx context.Context, before State, staged Staged) (UpdateResponse, error) {
	if err := g.stateShape(before); err != nil {
		return UpdateResponse{}, err
	}
	s := staged.State
	snapshot, err := g.Project(ctx, s)
	if err != nil {
		return UpdateResponse{}, err
	}
	if !snapshot.Validation.Valid || before.Revision != s.Revision {
		return UpdateResponse{}, errors.New("admin: invalid staged candidate")
	}
	changedKeys, restart := staged.ChangedKeys, staged.Restart
	actual := map[string]bool{}
	requiredTargets := map[string]bool{}
	desiredChanged := false
	for _, f := range g.Fields {
		old, value := before.Values[f.Key], s.Values[f.Key]
		if !sameResolved(old, value) {
			actual[f.Key] = true
		}
		if old.Present != value.Present || old.Value.value != value.Value.value {
			desiredChanged = true
			if f.RestartRequired {
				requiredTargets[f.ApplyTarget] = true
				if value.ApplyState != PendingRestart {
					return UpdateResponse{}, errors.New("admin: changed restart field is not pending")
				}
			}
		}
	}
	if (len(actual) == 0) != (before.Version == s.Version) {
		return UpdateResponse{}, errors.New("admin: inconsistent resulting version")
	}
	if len(changedKeys) != len(actual) {
		return UpdateResponse{}, errors.New("admin: invalid changed keys")
	}
	seen := map[string]bool{}
	for _, key := range changedKeys {
		if !actual[key] || seen[key] {
			return UpdateResponse{}, errors.New("admin: invalid changed keys")
		}
		seen[key] = true
	}
	if restart.Required != (len(restart.Targets) > 0) || (!desiredChanged && restart.Required) || (len(requiredTargets) > 0 && !restart.Required) {
		return UpdateResponse{}, errors.New("admin: invalid command restart projection")
	}
	targets := map[string]bool{}
	for _, target := range restart.Targets {
		if !validID(target) || targets[target] {
			return UpdateResponse{}, errors.New("admin: invalid apply targets")
		}
		targets[target] = true
	}
	for target := range requiredTargets {
		if !targets[target] {
			return UpdateResponse{}, errors.New("admin: missing affected restart target")
		}
	}
	return UpdateResponse{Snapshot: snapshot, ChangedKeys: append([]string{}, changedKeys...), RestartRequired: restart.Required, ApplyTargets: append([]string{}, restart.Targets...)}, nil
}
func sameResolved(a, b ResolvedValue) bool {
	sourceEqual := a.Source == nil && b.Source == nil || a.Source != nil && b.Source != nil && *a.Source == *b.Source
	return a.Present == b.Present && a.Value.value == b.Value.value && sourceEqual && a.Editable == b.Editable && a.ReadOnlyReason == b.ReadOnlyReason && a.HasOverride == b.HasOverride && a.ApplyState == b.ApplyState
}

// String deliberately omits privately resolved values from accidental formatting.
func (v ResolvedValue) String() string {
	return fmt.Sprintf("ResolvedValue{present:%t, value:redacted}", v.Present)
}
func (s State) String() string { return "State{values:redacted}" }

// ValidateCandidate combines profile checks with the host's side-effect-free
// semantic rules. The callback may inspect kept secrets but must sanitize errors.
func (g Group) ValidateCandidate(ctx context.Context, s State) (Validation, error) {
	result, err := g.Validate(s)
	if err != nil {
		return Validation{}, err
	}
	if g.SemanticValidation == nil {
		return result, nil
	}
	semantic, err := g.SemanticValidation(ctx, cloneValues(s.Values))
	if err != nil {
		return Validation{}, errors.New("admin: semantic validation unavailable")
	}
	if semantic.Valid != (len(semantic.Errors) == 0) {
		return Validation{}, errors.New("admin: inconsistent semantic validation")
	}
	for _, e := range semantic.Errors {
		known := e.Path == ""
		for _, f := range g.Fields {
			known = known || e.Path == pointer(f.Key)
		}
		if !known || !nonempty(e.Code) || !nonempty(e.Message) {
			return Validation{}, errors.New("admin: invalid semantic validation error")
		}
	}
	result.Errors = append(result.Errors, semantic.Errors...)
	result.Valid = len(result.Errors) == 0
	return result, nil
}
func (v ResolvedValue) GoString() string { return v.String() }
func (s State) GoString() string         { return s.String() }

func cloneValues(values map[string]ResolvedValue) map[string]ResolvedValue {
	copied := make(map[string]ResolvedValue, len(values))
	for key, v := range values {
		v.Source = copyPtr(v.Source)
		copied[key] = v
	}
	return copied
}
