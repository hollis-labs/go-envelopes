package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

var ctx = context.Background()

func text(v string) Scalar {
	s, err := String(v)
	if err != nil {
		panic(err)
	}
	return s
}
func num(v float64) Scalar {
	s, err := Number(v)
	if err != nil {
		panic(err)
	}
	return s
}
func ptr[T any](v T) *T { return &v }

// An inert adapter is enough for declarations; discovery must not invoke it.
type inert struct{}

func (inert) Read(context.Context) (State, error)             { panic("discovery read") }
func (inert) Preview(context.Context, Changes) (State, error) { panic("discovery preview") }
func (inert) WithTransaction(context.Context, func(GroupTransaction) error) error {
	panic("discovery transaction")
}
func group() Group {
	return Group{ID: "prefs", Label: "Preferences", Scope: Scope{Kind: "app", ID: "demo"}, Fields: []Field{
		{Key: "url", Type: StringType, Editable: true, Required: true, Default: ptr(text("https://example.test")), Pattern: `^https?://`, RestartRequired: true, ApplyTarget: "demo"},
		{Key: "token", Type: StringType, Editable: true, Secret: true, Required: true},
		{Key: "enabled", Type: BooleanType, Editable: true},
		{Key: "count", Type: IntegerType, Editable: true, Minimum: ptr(0.0)},
		{Key: "locked", Type: StringType, ReadOnlyReason: "Deployment-owned"},
	}, Resolution: Resolution{Precedence: []SourceKind{OverrideSource, EnvSource, FileSource, DefaultSource}, WriteLayer: OverrideSource}, Capabilities: Capabilities{CanRead: true, CanValidate: true, CanUpdate: true, CanReset: true}, Backend: inert{}}
}
func state() State {
	values := map[string]ResolvedValue{}
	for key, value := range map[string]Scalar{"url": text("https://example.test"), "token": text("synthetic-sensitive"), "enabled": Boolean(false), "count": num(0), "locked": text("deployment")} {
		values[key] = ResolvedValue{Present: true, Value: value, Source: &Source{Kind: DefaultSource, Label: "Declared default"}, Editable: true, ApplyState: Active}
	}
	v := values["token"]
	v.Source = &Source{Kind: OverrideSource, Label: "Secret override"}
	v.HasOverride = true
	values["token"] = v
	v = values["locked"]
	v.Editable = false
	v.ReadOnlyReason = "Deployment-owned"
	v.Source = &Source{Kind: EnvSource, Label: "Deployment environment"}
	values["locked"] = v
	return State{Revision: "r1", Version: "v1", Values: values}
}
func TestScalarPreservesTypesAndRejectsInvalid(t *testing.T) {
	for _, input := range []string{`false`, `0`, `""`, `1.5`, `1e2`, `-9007199254740991`} {
		var s Scalar
		if err := json.Unmarshal([]byte(input), &s); err != nil {
			t.Fatal(input, err)
		}
		b, err := json.Marshal(s)
		if err != nil || string(b) == "null" {
			t.Fatal(input, string(b), err)
		}
	}
	for _, input := range []string{`null`, `{}`, `[]`, `1e999`, `"x" false`} {
		var s Scalar
		if err := json.Unmarshal([]byte(input), &s); err == nil {
			t.Fatal("accepted", input)
		}
	}
	for _, input := range []string{`9007199254740992`, `9007199254740991.1`, `1.0000000000000001`, `1e-999`} {
		var s Scalar
		err := json.Unmarshal([]byte(input), &s)
		if err == nil && s.valid(IntegerType) {
			t.Fatal("coerced to integer", input)
		}
	}
	if _, err := Integer(MaxSafeInteger + 1); err == nil {
		t.Fatal("unsafe integer")
	}
	if _, err := Number(math.Inf(1)); err == nil {
		t.Fatal("infinite number")
	}
	if _, err := String(string([]byte{0xff})); err == nil {
		t.Fatal("bad UTF-8")
	}
	if _, err := json.Marshal(Scalar{}); err == nil {
		t.Fatal("zero scalar silently became null")
	}
}
func TestDeclarationBuiltOnceDetachedAndDiscoveryCheap(t *testing.T) {
	g := group()
	d := Definition{App: App{ID: "demo", Label: "Demo"}, Revision: "r1", BasePath: "/sysop", Groups: []Group{g}, Health: []HealthResource{{ID: "process", Label: "Processes", PollIntervalMS: 1000, StaleAfterMS: 2000, Read: func(context.Context) (HealthObservation, error) { panic("probe") }}}}
	r, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	m := r.Manifest()
	decl := m.Settings[0]
	if decl.Section != "settings" || decl.Schema.AdditionalProperties || decl.Schema.Properties["token"].WriteOnly != true || decl.Schema.Properties["locked"].ReadOnly != true {
		t.Fatal("bad derived schema")
	}
	if decl.Fields["token"].Secret != true || decl.Endpoints.Update.Path != "/sysop/admin/settings/prefs/update" || decl.Endpoints.Read.Method != "GET" {
		t.Fatal("bad derived wire metadata")
	}
	d.Groups[0].Fields[0].Key = "changed"
	*d.Groups[0].Fields[0].Default = text("mutated")
	m.Settings[0].Schema.Properties["url"] = Property{Type: BooleanType}
	m.Settings[0].Resolution.Precedence[0] = FileSource
	got, ok := r.Group("prefs")
	if !ok || got.Fields[0].Key != "url" || got.Fields[0].Default.Value() != "https://example.test" {
		t.Fatal("caller mutated registry")
	}
	*got.Fields[0].Default = text("again")
	if r.Manifest().Settings[0].Schema.Properties["url"].Default.Value() != "https://example.test" || r.Manifest().Settings[0].Resolution.Precedence[0] != OverrideSource {
		t.Fatal("manifest mutable")
	}
	bytes, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"stats":[]`, `"series":[]`, `"diagnostics":[]`} {
		if !strings.Contains(string(bytes), key) {
			t.Fatal("unsupported array missing", key)
		}
	}
	empty, err := New(Definition{App: App{"demo", "Demo"}, Revision: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ = json.Marshal(empty.Manifest())
	if !strings.Contains(string(bytes), `"settings":[]`) || !strings.Contains(string(bytes), `"health":[]`) {
		t.Fatal(string(bytes))
	}
}
func TestInvalidDeclarationsFailClosed(t *testing.T) {
	tests := map[string]func(*Group){
		"scope":                  func(g *Group) { g.Scope.Kind = "plugin" },
		"subject":                func(g *Group) { g.Scope.ID = "other" },
		"duplicate":              func(g *Group) { g.Fields = append(g.Fields, g.Fields[0]) },
		"secret default":         func(g *Group) { g.Fields[1].Default = ptr(text("synthetic")) },
		"secret enum":            func(g *Group) { g.Fields[1].Enum = []Scalar{} },
		"secret number":          func(g *Group) { g.Fields[1].Type = NumberType },
		"locked reason":          func(g *Group) { g.Fields[4].ReadOnlyReason = "" },
		"restart target":         func(g *Group) { g.Fields[0].ApplyTarget = "" },
		"unexpected target":      func(g *Group) { g.Fields[2].ApplyTarget = "demo" },
		"no read":                func(g *Group) { g.Capabilities.CanRead = false },
		"no validate":            func(g *Group) { g.Capabilities.CanValidate = false },
		"missing backend":        func(g *Group) { g.Backend = nil },
		"bad layer":              func(g *Group) { g.Resolution.Precedence = append(g.Resolution.Precedence, DefaultSource) },
		"wrong write layer":      func(g *Group) { g.Resolution.WriteLayer = FileSource },
		"type":                   func(g *Group) { g.Fields[0].Type = "array" },
		"enum duplicates":        func(g *Group) { g.Fields[0].Enum = []Scalar{text("x"), text("x")} },
		"enum type":              func(g *Group) { g.Fields[0].Enum = []Scalar{Boolean(false)} },
		"bad numeric constraint": func(g *Group) { g.Fields[0].Minimum = ptr(0.0) },
		"nonfinite":              func(g *Group) { g.Fields[3].Maximum = ptr(math.NaN()) },
		"contradictory limits":   func(g *Group) { g.Fields[3].Maximum = ptr(-1.0) },
		"negative length":        func(g *Group) { g.Fields[0].MinLength = ptr(-1) },
		"lookaround":             func(g *Group) { g.Fields[0].Pattern = `(?=x)x` },
		"readonly mutation": func(g *Group) {
			for i := range g.Fields {
				g.Fields[i].Editable = false
				g.Fields[i].ReadOnlyReason = "Locked"
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			g := group()
			mutate(&g)
			if _, err := New(Definition{App: App{"demo", "Demo"}, Revision: "r1", Groups: []Group{g}}); err == nil {
				t.Fatal("accepted invalid definition")
			}
		})
	}
	for _, path := range []string{"/../x", "//remote", "/a/./b", "/a%2fb", "/a?x", "/a#x", "/a/", "https://remote", "/a\\b"} {
		if SafePath(path) {
			t.Fatal("unsafe path accepted", path)
		}
	}
}
func TestPortablePatternSubset(t *testing.T) {
	for _, p := range []string{`^https?://`, `^[a-zA-Z0-9._-]{1,20}$`, `(yes|no)`, `[.]`, `[^a]`, `a\+b`} {
		if _, err := portablePattern(p); err != nil {
			t.Fatal("allowed pattern rejected", p)
		}
	}
	for _, p := range []string{`(?i)x`, `(?=x)`, `(?!x)`, `(?<=x)`, `(x)\1`, `\w+`, `\p{L}`, `.`, `.*`, `a+?`, `[[:alpha:]]`, `\bword`, `x{1,1001}`, `é`, `a}`, `{2}`, `a{b}`, `[[]`, `a]`} {
		if _, err := portablePattern(p); err == nil {
			t.Fatal("unsupported pattern accepted", p)
		}
	}
}
func TestProjectionSecretsAbsenceAndRecovery(t *testing.T) {
	g := group()
	s := state()
	snapshot, err := g.Project(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "synthetic-sensitive") || snapshot.Values["token"].Value != nil || snapshot.Values["token"].SecretPresent == nil || !*snapshot.Values["token"].SecretPresent {
		t.Fatal("secret disclosed or presence lost")
	}
	if _, err := json.Marshal(s); err == nil {
		t.Fatal("private state marshaled")
	}
	if _, err := json.Marshal(s.Values["token"]); err == nil {
		t.Fatal("private secret marshaled")
	}
	if strings.Contains(fmt.Sprintf("%v %#v", s, s), "synthetic-sensitive") || strings.Contains(fmt.Sprintf("%v %#v", s.Values["token"], s.Values["token"]), "synthetic-sensitive") {
		t.Fatal("private values leaked via formatting")
	}
	s.Values["token"] = ResolvedValue{Editable: true, ApplyState: UnknownApply}
	snapshot, err = g.Project(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	v := snapshot.Values["token"]
	if v.Source != nil || v.Value != nil || *v.SecretPresent || snapshot.Validation.Valid {
		t.Fatal("absent required secret was initialized or lost validation")
	}
	// Even a declared default must not initialize an absent desired value.
	s.Values["url"] = ResolvedValue{Editable: true, ApplyState: UnknownApply}
	snapshot, err = g.Project(ctx, s)
	if err != nil || snapshot.Values["url"].Present || snapshot.Values["url"].Value != nil {
		t.Fatal("default initialized missing config")
	}
}
func TestMalformedSnapshotMetadataRejected(t *testing.T) {
	for name, change := range map[string]func(*State){
		"source": func(s *State) {
			v := s.Values["url"]
			v.Source = &Source{Kind: "unknown", Label: "Safe"}
			s.Values["url"] = v
		},
		"missing source":  func(s *State) { v := s.Values["url"]; v.Source = nil; s.Values["url"] = v },
		"apply":           func(s *State) { v := s.Values["url"]; v.ApplyState = ""; s.Values["url"] = v },
		"permission":      func(s *State) { v := s.Values["locked"]; v.Editable = true; s.Values["locked"] = v },
		"reason":          func(s *State) { v := s.Values["locked"]; v.ReadOnlyReason = ""; s.Values["locked"] = v },
		"missing":         func(s *State) { delete(s.Values, "count") },
		"unknown field":   func(s *State) { s.Values["extra"] = ResolvedValue{} },
		"wrong type":      func(s *State) { v := s.Values["count"]; v.Value = text("1"); s.Values["count"] = v },
		"absent source":   func(s *State) { v := s.Values["url"]; v.Present = false; v.Value = Scalar{}; s.Values["url"] = v },
		"invalid version": func(s *State) { s.Version = `bad"tag` },
	} {
		t.Run(name, func(t *testing.T) {
			s := state()
			change(&s)
			if _, err := group().Project(ctx, s); err == nil {
				t.Fatal("accepted malformed snapshot")
			}
		})
	}
}
func TestCompleteValidationSecretsUnicodeAndSemanticRules(t *testing.T) {
	g := group()
	g.Fields[1].MinLength = ptr(2)
	s := state()
	v := s.Values["token"]
	v.Value = text("🔒")
	s.Values["token"] = v
	result, err := g.Validate(s)
	if err != nil || result.Valid || result.Errors[0].Path != "/token" || strings.Contains(result.Errors[0].Message, "🔒") {
		t.Fatal(result, err)
	}
	v.Value = text("")
	s.Values["token"] = v
	result, _ = g.Validate(s)
	if result.Valid {
		t.Fatal("empty string became keep or unset")
	}
	g.Fields[1].MinLength = nil
	result, _ = g.Validate(s)
	if !result.Valid {
		t.Fatal("unconstrained empty secret rejected")
	}
	g.SemanticValidation = func(_ context.Context, values map[string]ResolvedValue) (Validation, error) {
		if values["token"].Value.Value() != "" {
			t.Fatal("semantic validator did not get private candidate")
		}
		return Validation{Errors: []ValidationError{{Path: "", Code: "cross_field", Message: "The configuration combination is unsupported."}}}, nil
	}
	result, err = g.ValidateCandidate(ctx, s)
	if err != nil || result.Valid || result.Errors[0].Code != "cross_field" {
		t.Fatal(result, err)
	}
	g.SemanticValidation = func(context.Context, map[string]ResolvedValue) (Validation, error) {
		return Validation{}, errors.New("synthetic-sensitive")
	}
	_, err = g.Project(ctx, s)
	if err == nil || strings.Contains(err.Error(), "synthetic-sensitive") {
		t.Fatal("raw host error escaped", err)
	}
}
func TestChangesPermissionsAndExplicitUnset(t *testing.T) {
	g := group()
	s := state()
	empty := Changes{Set: map[string]Scalar{}, Unset: []string{}}
	if f := g.CheckChanges(s, empty); f != nil {
		t.Fatal(f)
	}
	if f := g.CheckChanges(s, Changes{Set: map[string]Scalar{"token": text("")}, Unset: []string{}}); f != nil {
		t.Fatal("empty not accepted as typed replacement", f)
	}
	for name, c := range map[string]Changes{
		"nil": {}, "unknown": {Set: map[string]Scalar{"extra": text("x")}, Unset: []string{}}, "locked": {Set: map[string]Scalar{"locked": text("x")}, Unset: []string{}}, "type": {Set: map[string]Scalar{"count": text("1")}, Unset: []string{}}, "overlap": {Set: map[string]Scalar{"token": text("x")}, Unset: []string{"token"}}, "duplicate": {Set: map[string]Scalar{}, Unset: []string{"token", "token"}},
	} {
		t.Run(name, func(t *testing.T) {
			if g.CheckChanges(s, c) == nil {
				t.Fatal("bad changes accepted")
			}
		})
	}
	g.Capabilities.CanReset = false
	if f := g.CheckChanges(s, Changes{Set: map[string]Scalar{}, Unset: []string{"token"}}); f == nil || f.Code != Forbidden {
		t.Fatal("can_reset bypass", f)
	}
	// Unset cannot prove its fallback locally; the host-resolved absent candidate fails complete validation.
	g = group()
	fallback := state()
	fallback.Values["token"] = ResolvedValue{Editable: true, ApplyState: UnknownApply}
	v, err := g.Validate(fallback)
	if err != nil || v.Valid {
		t.Fatal("invalid required fallback accepted")
	}
}
func TestCommandRestartProjectionAndNoopPending(t *testing.T) {
	g := group()
	before := state()
	v := before.Values["url"]
	v.ApplyState = PendingRestart
	before.Values["url"] = v
	response, err := g.UpdateResult(ctx, before, Staged{State: before})
	if err != nil || response.RestartRequired || len(response.ApplyTargets) != 0 || response.Snapshot.Values["url"].ApplyState != PendingRestart {
		t.Fatal("no-op lost pending state or claimed new restart", response, err)
	}
	// A new desired change must name the host-supplied target and mark the value pending.
	after := state()
	after.Version = "v2"
	v = after.Values["url"]
	v.Value = text("https://new.test")
	v.ApplyState = PendingRestart
	after.Values["url"] = v
	staged := Staged{State: after, ChangedKeys: []string{"url"}, Restart: Restart{Required: true, Targets: []string{"demo"}}}
	if _, err := g.UpdateResult(ctx, before, staged); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Staged){"empty targets": func(s *Staged) { s.Restart.Targets = nil }, "false with target": func(s *Staged) { s.Restart.Required = false }, "duplicate targets": func(s *Staged) { s.Restart.Targets = append(s.Restart.Targets, "demo") }, "missing affected target": func(s *Staged) { s.Restart.Targets = []string{"other"} }, "unknown key": func(s *Staged) { s.ChangedKeys = []string{"extra"} }, "reused version": func(s *Staged) { s.State.Version = "v1" }} {
		t.Run(name, func(t *testing.T) {
			bad := staged
			bad.Restart.Targets = append([]string{}, staged.Restart.Targets...)
			mutate(&bad)
			if _, err := g.UpdateResult(ctx, before, bad); err == nil {
				t.Fatal("invalid staged result accepted")
			}
		})
	}
	if _, err := g.UpdateResult(ctx, before, Staged{State: before, Restart: Restart{Required: true, Targets: []string{"demo"}}}); err == nil {
		t.Fatal("outstanding pending work emitted as command result")
	}
	tag, err := before.ETag()
	if err != nil || tag != `"v1"` {
		t.Fatal(tag, err)
	}
}
func TestObservationsMissingUnhealthyAndErrors(t *testing.T) {
	unhealthy := HealthObservation{ObservedAt: "2026-10-02T00:00:00Z", Status: Unhealthy, Checks: []HealthCheck{{ID: "db", Label: "Database", Status: Unhealthy}}}
	if err := unhealthy.Validate(); err != nil {
		t.Fatal("unhealthy misclassified as unavailable", err)
	}
	unhealthy.Status = "alien"
	if unhealthy.Validate() == nil {
		t.Fatal("unknown enum accepted")
	}
	o := StatObservation{ObservedAt: "2026-10-02T00:00:00Z"}
	if err := o.Validate(Count); err != nil {
		t.Fatal("missing sample rejected")
	}
	b, _ := json.Marshal(o)
	if !strings.Contains(string(b), `"value":null`) {
		t.Fatal(string(b))
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), -0.1, 1.1} {
		o.Value = ptr(value)
		if o.Validate(Ratio) == nil {
			t.Fatal("bad ratio")
		}
	}
	for code, want := range map[string]int{MalformedInput: 400, Unauthenticated: 401, Forbidden: 403, UnknownResource: 404, ManifestChanged: 409, ValueConflict: 412, ValidationFailed: 422, PreconditionRequired: 428, Unsupported: 501, BackendUnavailable: 503, "arbitrary": 503} {
		if got := (&Failure{Code: code}).StatusCode(); got != want {
			t.Fatal(code, got, want)
		}
	}
}

func TestObservationDeclarationsAndReadonlyCapabilities(t *testing.T) {
	g := group()
	g.Capabilities = Capabilities{CanRead: true}
	g.Resolution.WriteLayer = ""
	r, err := New(Definition{App: App{"demo", "Demo"}, Revision: "r1", Groups: []Group{g}, Stats: []StatResource{{ID: "active", Label: "Active processes", Unit: Count, Kind: Gauge, PollIntervalMS: 1000, StaleAfterMS: 2000, Read: func(context.Context) (StatObservation, error) { panic("discovery stat probe") }}}})
	if err != nil {
		t.Fatal(err)
	}
	endpoints := r.Manifest().Settings[0].Endpoints
	if endpoints.Read == nil || endpoints.Validate != nil || endpoints.Update != nil || endpoints.Reset != nil {
		t.Fatal("unsupported endpoint advertised")
	}
	if _, ok := r.Stat("active"); !ok {
		t.Fatal("stat callback lost")
	}
	if _, ok := r.Stat("missing"); ok {
		t.Fatal("unknown stat")
	}
	if _, ok := r.Health("missing"); ok {
		t.Fatal("unknown health")
	}
	if _, ok := r.Group("missing"); ok {
		t.Fatal("unknown group")
	}
	for _, bad := range []StatResource{{ID: "bad", Label: "Bad", Unit: Ratio, Kind: Gauge, PollIntervalMS: 1000, StaleAfterMS: 2000}, {ID: "bad", Label: "Bad", Unit: "invented", Kind: Gauge, PollIntervalMS: 1000, StaleAfterMS: 2000, Read: func(context.Context) (StatObservation, error) { return StatObservation{}, nil }}} {
		if _, err := New(Definition{App: App{"demo", "Demo"}, Revision: "r1", Stats: []StatResource{bad}}); err == nil {
			t.Fatal("invalid observation advertised")
		}
	}
	h := HealthObservation{ObservedAt: "2026-10-02T00:00:00+00:00", Status: Healthy, Checks: []HealthCheck{}}
	if h.Validate() == nil {
		t.Fatal("non-Z timestamp accepted")
	}
	h.ObservedAt = "2026-10-02T00:00:00Z"
	h.Checks = []HealthCheck{{ID: "db", Label: "DB", Status: Healthy}, {ID: "db", Label: "DB", Status: Unhealthy}}
	if h.Validate() == nil {
		t.Fatal("duplicate checks accepted")
	}
}
func TestSemanticValidatorCannotMutateCandidate(t *testing.T) {
	g := group()
	s := state()
	g.SemanticValidation = func(_ context.Context, v map[string]ResolvedValue) (Validation, error) {
		old := v["url"]
		old.Source.Label = "Mutated"
		old.Value = text("bad")
		v["url"] = old
		return Validation{Valid: true}, nil
	}
	snapshot, err := g.Project(ctx, s)
	if err != nil || snapshot.Values["url"].Value.Value() != "https://example.test" || s.Values["url"].Source.Label == "Mutated" {
		t.Fatal("validator changed candidate", err)
	}
	for _, bad := range []Validation{{Valid: true, Errors: []ValidationError{{Code: "bad", Message: "Failure"}}}, {Errors: []ValidationError{{Path: "/undeclared", Code: "bad", Message: "Failure"}}}, {Errors: []ValidationError{{Path: "/url", Code: "", Message: "Failure"}}}} {
		g.SemanticValidation = func(context.Context, map[string]ResolvedValue) (Validation, error) { return bad, nil }
		if _, err := g.Project(ctx, s); err == nil {
			t.Fatal("inconsistent semantic errors accepted")
		}
	}
}
func TestSourceOnlyChangeDoesNotRequireRestart(t *testing.T) {
	g := group()
	before := state()
	after := state()
	after.Version = "v2"
	v := after.Values["url"]
	v.Source = &Source{Kind: OverrideSource, Label: "App override"}
	v.HasOverride = true
	after.Values["url"] = v
	response, err := g.UpdateResult(ctx, before, Staged{State: after, ChangedKeys: []string{"url"}})
	if err != nil || response.RestartRequired {
		t.Fatal("unchanged desired value incorrectly requires restart", err)
	}
	if _, err := g.UpdateResult(ctx, before, Staged{State: after, ChangedKeys: []string{"url"}, Restart: Restart{Required: true, Targets: []string{"demo"}}}); err == nil {
		t.Fatal("metadata-only change requires restart")
	}
	// Backend may conservatively need its own restart for a changed live field.
	after = state()
	after.Version = "v2"
	v = after.Values["enabled"]
	v.Value = Boolean(true)
	v.ApplyState = UnknownApply
	after.Values["enabled"] = v
	if _, err := g.UpdateResult(ctx, before, Staged{State: after, ChangedKeys: []string{"enabled"}, Restart: Restart{Required: true, Targets: []string{"demo"}}}); err != nil {
		t.Fatal("conservative restart rejected", err)
	}
}

// Core key validation is revision-independent; transports must check the
// command/declaration revision before calling CheckChanges.
func TestCheckChangesDoesNotCheckDeclarationRevision(t *testing.T) {
	g := group()
	for _, revision := range []string{"r1", "old"} {
		s := state()
		s.Revision = revision
		unknown := Changes{Set: map[string]Scalar{"unknown": text("x")}, Unset: []string{}}
		if f := g.CheckChanges(s, unknown); f == nil || f.Code != MalformedInput || f.StatusCode() != 400 {
			t.Fatal("core must reject unknown keys independently of revision", f)
		}
		valid := Changes{Set: map[string]Scalar{"url": text("https://next.test")}, Unset: []string{}}
		if f := g.CheckChanges(s, valid); f != nil {
			t.Fatal("core unexpectedly checks declaration revision", f)
		}
	}
}
