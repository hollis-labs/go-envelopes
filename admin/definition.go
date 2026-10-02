package admin

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Field is declared once; the builder derives schema and field metadata together.
// Default is an annotation only. Secret fields cannot declare defaults or enums.
type Field struct {
	Key             string
	Type            ScalarType
	Title           string
	Description     string
	Required        bool
	Default         *Scalar
	Enum            []Scalar
	Minimum         *float64
	Maximum         *float64
	MinLength       *int
	MaxLength       *int
	Pattern         string
	Editable        bool
	Secret          bool
	ReadOnlyReason  string
	RestartRequired bool
	ApplyTarget     string
}
type Group struct {
	ID                 string
	Label              string
	Scope              Scope
	Fields             []Field
	Resolution         Resolution
	Capabilities       Capabilities
	Backend            GroupBackend
	SemanticValidation SemanticValidator
}
type Definition struct {
	App      App
	Revision string
	// BasePath is the deployment prefix, e.g. /sysop; endpoints append /admin/...
	BasePath string
	Groups   []Group
	Health   []HealthResource
	Stats    []StatResource
}
type Property struct {
	Type        ScalarType `json:"type"`
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	Default     *Scalar    `json:"default,omitempty"`
	Enum        []Scalar   `json:"enum,omitempty"`
	Minimum     *float64   `json:"minimum,omitempty"`
	Maximum     *float64   `json:"maximum,omitempty"`
	MinLength   *int       `json:"minLength,omitempty"`
	MaxLength   *int       `json:"maxLength,omitempty"`
	Pattern     string     `json:"pattern,omitempty"`
	ReadOnly    bool       `json:"readOnly,omitempty"`
	WriteOnly   bool       `json:"writeOnly,omitempty"`
}
type Schema struct {
	Schema               string              `json:"$schema"`
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties"`
	Required             []string            `json:"required"`
	AdditionalProperties bool                `json:"additionalProperties"`
}
type FieldDeclaration struct {
	Editable        bool   `json:"editable"`
	Secret          bool   `json:"secret"`
	RestartRequired bool   `json:"restart_required"`
	ApplyTarget     string `json:"apply_target,omitempty"`
	ReadOnlyReason  string `json:"read_only_reason,omitempty"`
}
type Endpoints struct {
	Read     *Endpoint `json:"read,omitempty"`
	Validate *Endpoint `json:"validate,omitempty"`
	Update   *Endpoint `json:"update,omitempty"`
	Reset    *Endpoint `json:"reset,omitempty"`
}
type GroupDeclaration struct {
	ID           string                      `json:"id"`
	Label        string                      `json:"label"`
	Section      string                      `json:"section"`
	Scope        Scope                       `json:"scope"`
	Schema       Schema                      `json:"schema"`
	Fields       map[string]FieldDeclaration `json:"fields"`
	Resolution   Resolution                  `json:"resolution"`
	Capabilities Capabilities                `json:"capabilities"`
	Endpoints    Endpoints                   `json:"endpoints"`
}
type Manifest struct {
	ContractVersion int                      `json:"contract_version"`
	App             App                      `json:"app"`
	Revision        string                   `json:"revision"`
	Settings        []GroupDeclaration       `json:"settings"`
	Health          []ObservationDeclaration `json:"health"`
	Stats           []StatDeclaration        `json:"stats"`
	Series          []struct{}               `json:"series"`
	Diagnostics     []struct{}               `json:"diagnostics"`
}

// Registry is an immutable, validated declaration snapshot. It owns no state.
type Registry struct {
	manifest Manifest
	groups   []Group
	health   []HealthResource
	stats    []StatResource
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

func validID(v string) bool  { return idPattern.MatchString(v) }
func nonempty(v string) bool { return strings.TrimSpace(v) != "" }

// SafePath accepts canonical, origin-relative paths without queries, escaping,
// credentials, traversal, fragments or a cross-origin/relative spelling.
func SafePath(v string) bool {
	if !strings.HasPrefix(v, "/") || strings.ContainsAny(v, "%?#\\") {
		return false
	}
	for _, segment := range strings.Split(v[1:], "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		for _, c := range segment {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("._~-", c)) {
				return false
			}
		}
	}
	return true
}
func invalidDefinition() error { return errors.New("admin: invalid or unsupported declaration") }

// New validates and defensively copies declarations. It performs no backend read
// or probe. This implementation supports only scope {kind:app,id:app.id}.
func New(d Definition) (*Registry, error) {
	if !validID(d.App.ID) || !nonempty(d.App.Label) || !nonempty(d.Revision) || (d.BasePath != "" && !SafePath(d.BasePath)) {
		return nil, invalidDefinition()
	}
	r := &Registry{manifest: Manifest{ContractVersion: ContractVersion, App: d.App, Revision: d.Revision, Settings: []GroupDeclaration{}, Health: []ObservationDeclaration{}, Stats: []StatDeclaration{}, Series: []struct{}{}, Diagnostics: []struct{}{}}}
	seen := map[string]bool{}
	for _, g := range d.Groups {
		if seen[g.ID] {
			return nil, invalidDefinition()
		}
		seen[g.ID] = true
		decl, err := g.declaration(d.App, d.BasePath)
		if err != nil {
			return nil, err
		}
		r.groups = append(r.groups, cloneGroup(g))
		r.manifest.Settings = append(r.manifest.Settings, decl)
	}
	seen = map[string]bool{}
	for _, h := range d.Health {
		decl := observationDeclaration(h.ID, h.Label, d.BasePath+"/admin/health/"+h.ID, h.PollIntervalMS, h.StaleAfterMS)
		if seen[h.ID] || h.Read == nil || !validObservationDeclaration(decl) {
			return nil, invalidDefinition()
		}
		seen[h.ID] = true
		r.health = append(r.health, h)
		r.manifest.Health = append(r.manifest.Health, decl)
	}
	seen = map[string]bool{}
	for _, s := range d.Stats {
		decl := StatDeclaration{ObservationDeclaration: observationDeclaration(s.ID, s.Label, d.BasePath+"/admin/stats/"+s.ID, s.PollIntervalMS, s.StaleAfterMS), Unit: s.Unit, Kind: s.Kind}
		if seen[s.ID] || s.Read == nil || !validObservationDeclaration(decl.ObservationDeclaration) || !validUnit(s.Unit) || !validKind(s.Kind) {
			return nil, invalidDefinition()
		}
		seen[s.ID] = true
		r.stats = append(r.stats, s)
		r.manifest.Stats = append(r.manifest.Stats, decl)
	}
	return r, nil
}

// Manifest returns metadata only, not settings or observations. Returned data is
// detached; callers may filter it for an authorized context without changing r.
func (r *Registry) Manifest() Manifest {
	b, _ := json.Marshal(r.manifest)
	var m Manifest
	_ = json.Unmarshal(b, &m)
	return m
}
func (r *Registry) Group(id string) (Group, bool) {
	for _, g := range r.groups {
		if g.ID == id {
			return cloneGroup(g), true
		}
	}
	return Group{}, false
}
func (r *Registry) Health(id string) (HealthResource, bool) {
	for _, h := range r.health {
		if h.ID == id {
			return h, true
		}
	}
	return HealthResource{}, false
}
func (r *Registry) Stat(id string) (StatResource, bool) {
	for _, s := range r.stats {
		if s.ID == id {
			return s, true
		}
	}
	return StatResource{}, false
}
func (g Group) declaration(app App, prefix string) (GroupDeclaration, error) {
	if !validID(g.ID) || !nonempty(g.Label) || g.Scope.Kind != "app" || g.Scope.ID != app.ID || !g.validProfile() {
		return GroupDeclaration{}, invalidDefinition()
	}
	caps := g.Capabilities
	if (caps.CanRead || caps.CanValidate || caps.CanUpdate || caps.CanReset) && g.Backend == nil {
		return GroupDeclaration{}, invalidDefinition()
	}
	if (caps.CanUpdate || caps.CanReset) && (!caps.CanRead || !caps.CanValidate || g.Resolution.WriteLayer != OverrideSource) {
		return GroupDeclaration{}, invalidDefinition()
	}
	if !caps.CanUpdate && !caps.CanReset && g.Resolution.WriteLayer != "" {
		return GroupDeclaration{}, invalidDefinition()
	}
	layers := map[SourceKind]bool{}
	for _, l := range g.Resolution.Precedence {
		if !validSource(l) || layers[l] {
			return GroupDeclaration{}, invalidDefinition()
		}
		layers[l] = true
	}
	if len(layers) == 0 || (g.Resolution.WriteLayer != "" && !layers[OverrideSource]) {
		return GroupDeclaration{}, invalidDefinition()
	}
	d := GroupDeclaration{ID: g.ID, Label: g.Label, Section: "settings", Scope: g.Scope, Capabilities: caps, Resolution: Resolution{Precedence: append([]SourceKind{}, g.Resolution.Precedence...), WriteLayer: g.Resolution.WriteLayer}, Schema: Schema{Schema: "https://json-schema.org/draft/2020-12/schema", Type: "object", Properties: map[string]Property{}, Required: []string{}}, Fields: map[string]FieldDeclaration{}}
	for _, f := range g.Fields {
		d.Schema.Properties[f.Key] = Property{Type: f.Type, Title: f.Title, Description: f.Description, Default: copyPtr(f.Default), Enum: append([]Scalar(nil), f.Enum...), Minimum: copyPtr(f.Minimum), Maximum: copyPtr(f.Maximum), MinLength: copyPtr(f.MinLength), MaxLength: copyPtr(f.MaxLength), Pattern: f.Pattern, ReadOnly: !f.Editable, WriteOnly: f.Secret}
		if f.Required {
			d.Schema.Required = append(d.Schema.Required, f.Key)
		}
		d.Fields[f.Key] = FieldDeclaration{Editable: f.Editable, Secret: f.Secret, RestartRequired: f.RestartRequired, ApplyTarget: f.ApplyTarget, ReadOnlyReason: f.ReadOnlyReason}
	}
	path := prefix + "/admin/settings/" + g.ID
	if caps.CanRead {
		d.Endpoints.Read = &Endpoint{Method: "GET", Path: path}
	}
	if caps.CanValidate {
		d.Endpoints.Validate = &Endpoint{Method: "POST", Path: path + "/validate"}
	}
	if caps.CanUpdate {
		d.Endpoints.Update = &Endpoint{Method: "POST", Path: path + "/update"}
	}
	if caps.CanReset {
		d.Endpoints.Reset = &Endpoint{Method: "POST", Path: path + "/reset"}
	}
	return d, nil
}
func copyPtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func cloneGroup(g Group) Group {
	g.Resolution.Precedence = append([]SourceKind(nil), g.Resolution.Precedence...)
	g.Fields = append([]Field(nil), g.Fields...)
	for i := range g.Fields {
		f := &g.Fields[i]
		f.Enum = append([]Scalar(nil), f.Enum...)
		f.Default = copyPtr(f.Default)
		f.Minimum = copyPtr(f.Minimum)
		f.Maximum = copyPtr(f.Maximum)
		f.MinLength = copyPtr(f.MinLength)
		f.MaxLength = copyPtr(f.MaxLength)
	}
	return g
}
