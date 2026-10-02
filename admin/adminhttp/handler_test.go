package adminhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hollis-labs/go-envelopes/admin"
	"github.com/hollis-labs/go-envelopes/admin/adminhttp"
)

func scalar(s string) admin.Scalar {
	v, err := admin.String(s)
	if err != nil {
		panic(err)
	}
	return v
}
func ptr[T any](v T) *T { return &v }
func copyState(s admin.State) admin.State {
	out := s
	out.Values = map[string]admin.ResolvedValue{}
	for key, v := range s.Values {
		if v.Source != nil {
			source := *v.Source
			v.Source = &source
		}
		out.Values[key] = v
	}
	return out
}

// The test adapter has a real rollback boundary for its simulated durable stores.
// No production storage or lifecycle implementation is supplied by the library.
type store struct {
	mu                                                                  sync.Mutex
	state                                                               admin.State
	fallback                                                            map[string]admin.ResolvedValue
	reads, previews, transactions, stages, commits, rollbacks, restarts int
	stageFailure, commitFailure, invalidTargets, differentStagedValue   bool
	previewChanged                                                      bool
	repeatCallback                                                      bool
	cancelStage                                                         context.CancelFunc
}

func newStore() *store {
	values := map[string]admin.ResolvedValue{}
	for key, value := range map[string]admin.Scalar{"url": scalar("https://initial.test"), "token": scalar("synthetic-private-initial"), "flag": admin.Boolean(false), "locked": scalar("deployment")} {
		values[key] = admin.ResolvedValue{Present: true, Value: value, Source: &admin.Source{Kind: admin.OverrideSource, Label: "App override"}, Editable: true, HasOverride: true, ApplyState: admin.Active}
	}
	v := values["locked"]
	v.Editable = false
	v.ReadOnlyReason = "Deployment-owned"
	v.Source = &admin.Source{Kind: admin.EnvSource, Label: "Deployment environment"}
	v.HasOverride = false
	values["locked"] = v
	return &store{state: admin.State{Revision: "r1", Version: "v1", Values: values}, fallback: map[string]admin.ResolvedValue{"url": {Present: true, Value: scalar("https://fallback.test"), Source: &admin.Source{Kind: admin.DefaultSource, Label: "Declared fallback"}, Editable: true, ApplyState: admin.Active}}}
}
func (s *store) Read(context.Context) (admin.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	return copyState(s.state), nil
}
func (s *store) resolve(c admin.Changes) admin.State {
	result := copyState(s.state)
	for key, value := range c.Set {
		v := result.Values[key]
		v.Value = value
		v.Present = true
		v.Source = &admin.Source{Kind: admin.OverrideSource, Label: "App override"}
		v.HasOverride = true
		result.Values[key] = v
	}
	for _, key := range c.Unset {
		if v, ok := s.fallback[key]; ok {
			result.Values[key] = v
		} else {
			result.Values[key] = admin.ResolvedValue{Editable: true, ApplyState: admin.UnknownApply}
		}
	}
	return result
}
func (s *store) Preview(_ context.Context, c admin.Changes) (admin.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.previews++
	v := s.resolve(c)
	if s.previewChanged {
		v.Version = "changed"
	}
	return v, nil
}
func (s *store) WithTransaction(_ context.Context, fn func(admin.GroupTransaction) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transactions++
	tx := &transaction{store: s}
	err := fn(tx)
	if err == nil && s.repeatCallback {
		err = fn(tx)
	}
	if err != nil {
		s.rollbacks++
		return err
	}
	if s.commitFailure {
		s.rollbacks++
		return errors.New("synthetic-private-commit-error")
	}
	if tx.staged != nil {
		s.state = copyState(*tx.staged)
		s.commits++
	}
	return nil
}

type transaction struct {
	store  *store
	staged *admin.State
}

func (tx *transaction) Current() (admin.State, error)                { return copyState(tx.store.state), nil }
func (tx *transaction) Resolve(c admin.Changes) (admin.State, error) { return tx.store.resolve(c), nil }
func (tx *transaction) Stage(c admin.Changes) (admin.Staged, error) {
	tx.store.stages++
	next := tx.store.resolve(c)
	changed := []string{}
	restart := admin.Restart{}
	for _, key := range []string{"url", "token", "flag", "locked"} {
		old, v := tx.store.state.Values[key], next.Values[key]
		desired := old.Present != v.Present || old.Value.Value() != v.Value.Value()
		if desired && key == "url" {
			v.ApplyState = admin.PendingRestart
			next.Values[key] = v
			restart = admin.Restart{Required: true, Targets: []string{"demo"}}
		}
		if !reflect.DeepEqual(old, v) {
			changed = append(changed, key)
		}
	}
	if len(changed) > 0 {
		next.Version = tx.store.state.Version + "x"
	}
	tx.staged = &next
	if tx.store.cancelStage != nil {
		tx.store.cancelStage()
	}
	if tx.store.stageFailure {
		return admin.Staged{}, errors.New("synthetic-private-stage-error")
	}
	if tx.store.invalidTargets {
		restart = admin.Restart{Required: true}
	}
	if tx.store.differentStagedValue {
		v := next.Values["token"]
		v.Value = scalar("synthetic-unrequested")
		next.Values["token"] = v
	}
	return admin.Staged{State: next, ChangedKeys: changed, Restart: restart}, nil
}
func declaration(s *store) admin.Definition {
	return admin.Definition{App: admin.App{ID: "demo", Label: "Demo"}, Revision: "r1", Groups: []admin.Group{{ID: "prefs", Label: "Preferences", Scope: admin.Scope{Kind: "app", ID: "demo"}, Fields: []admin.Field{
		{Key: "url", Type: admin.StringType, Editable: true, Required: true, Pattern: `^https?://`, RestartRequired: true, ApplyTarget: "demo"},
		{Key: "token", Type: admin.StringType, Editable: true, Secret: true, Required: true},
		{Key: "flag", Type: admin.BooleanType, Editable: true},
		{Key: "locked", Type: admin.StringType, ReadOnlyReason: "Deployment-owned"},
	}, Resolution: admin.Resolution{Precedence: []admin.SourceKind{admin.OverrideSource, admin.EnvSource, admin.DefaultSource}, WriteLayer: admin.OverrideSource}, Capabilities: admin.Capabilities{CanRead: true, CanValidate: true, CanUpdate: true, CanReset: true}, Backend: s}}}
}
func configuration(t *testing.T, d admin.Definition) adminhttp.Config {
	t.Helper()
	r, err := admin.New(d)
	if err != nil {
		t.Fatal(err)
	}
	return adminhttp.Config{BasePath: d.BasePath, Authorize: func(_ *http.Request, _ adminhttp.Resource) error { return nil }, Resolve: func(*http.Request) (*admin.Registry, error) { return r, nil }, ProtectCommand: func(*http.Request) error { return nil }}
}
func newHandler(t *testing.T, s *store) http.Handler {
	t.Helper()
	h, err := adminhttp.NewHandler(configuration(t, declaration(s)))
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func request(h http.Handler, method, path, body, tag string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if tag != "" {
		r.Header.Set("If-Match", tag)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func check(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("response missing protocol headers")
	}
	if code != "" {
		var response admin.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Error.Code != code {
			t.Fatal("unexpected error contract", w.Body.String(), err)
		}
	}
	if strings.Contains(w.Body.String(), "synthetic-private") {
		t.Fatal("secret/raw host error disclosed")
	}
}

const empty = `{"revision":"r1","set":{},"unset":[]}`
const change = `{"revision":"r1","set":{"url":"https://next.test"},"unset":[]}`

func TestDiscoveryAndReadRedactionNoProbeFanout(t *testing.T) {
	s := newStore()
	h := newHandler(t, s)
	check(t, request(h, "GET", "/admin/manifest", "", ""), 200, "")
	if s.reads != 0 || s.transactions != 0 || s.previews != 0 {
		t.Fatal("discovery probed or mutated")
	}
	w := request(h, "GET", "/admin/settings/prefs", "", "")
	check(t, w, 200, "")
	var snapshot admin.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("ETag") != `"v1"` || snapshot.Values["token"].Value != nil || snapshot.Values["token"].SecretPresent == nil || !*snapshot.Values["token"].SecretPresent {
		t.Fatal("secret presence/tag invalid")
	}
	if s.commits != 0 || s.restarts != 0 {
		t.Fatal("read applied settings")
	}
}
func TestPreconditionAndRevisionPrecedence(t *testing.T) {
	tests := []struct {
		name, body, tag string
		status          int
		code            string
	}{
		{"missing", change, "", 428, admin.PreconditionRequired},
		{"stale", change, `"old"`, 412, admin.ValueConflict},
		{"weak", change, `W/"v1"`, 412, admin.ValueConflict},
		{"wildcard", change, `*`, 400, admin.MalformedInput},
		{"multiple", change, `"v1", "other"`, 400, admin.MalformedInput},
		{"manifest beats value", strings.Replace(change, "r1", "old", 1), `"old"`, 409, admin.ManifestChanged},
		{"manifest beats missing", strings.Replace(change, "r1", "old", 1), "", 409, admin.ManifestChanged},
		{"malformed beats revision", `{"revision":"old","set":{},"unset":["url","url"]}`, `"old"`, 400, admin.MalformedInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()
			before := copyState(s.state)
			w := request(newHandler(t, s), "POST", "/admin/settings/prefs/update", tt.body, tt.tag)
			check(t, w, tt.status, tt.code)
			if s.stages != 0 || s.commits != 0 || !reflect.DeepEqual(s.state, before) {
				t.Fatal("failed precondition changed state")
			}
		})
	}
	s := newStore()
	s.state.Revision = "r2"
	check(t, request(newHandler(t, s), "POST", "/admin/settings/prefs/update", change, `"v1"`), 409, admin.ManifestChanged)
}
func TestValidateFullCandidateAndResetFallback(t *testing.T) {
	s := newStore()
	h := newHandler(t, s)
	// Completed invalid validation is 200, never a successful persistence.
	w := request(h, "POST", "/admin/settings/prefs/validate", `{"revision":"r1","set":{"url":"invalid"},"unset":[]}`, "")
	check(t, w, 200, "")
	var result admin.Validation
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Valid || len(result.Errors) == 0 {
		t.Fatal(w.Body.String(), err)
	}
	if s.stages != 0 || s.commits != 0 || s.restarts != 0 {
		t.Fatal("validate wrote or applied")
	}
	check(t, request(h, "POST", "/admin/settings/prefs/validate", strings.Replace(empty, "r1", "old", 1), ""), 409, admin.ManifestChanged)
	// Required secret with no fallback cannot be removed; reset is validation-preserving.
	before := copyState(s.state)
	w = request(h, "POST", "/admin/settings/prefs/reset", `{"revision":"r1","keys":["token"]}`, `"v1"`)
	check(t, w, 422, admin.ValidationFailed)
	if !reflect.DeepEqual(s.state, before) || s.commits != 0 {
		t.Fatal("invalid reset erased override")
	}
	// A real fallback resolves and validates complete desired config before commit.
	w = request(h, "POST", "/admin/settings/prefs/reset", `{"revision":"r1","keys":["url"]}`, `"v1"`)
	check(t, w, 200, "")
	if s.state.Values["url"].Value.Value() != "https://fallback.test" || s.state.Values["url"].HasOverride || s.state.Values["token"].Value.Value() != "synthetic-private-initial" {
		t.Fatal("reset touched wrong layer/key")
	}
}
func TestSecretsEmptyRealValueAndOmissionKeep(t *testing.T) {
	s := newStore()
	h := newHandler(t, s)
	w := request(h, "POST", "/admin/settings/prefs/update", `{"revision":"r1","set":{"token":""},"unset":[]}`, `"v1"`)
	check(t, w, 200, "")
	if !s.state.Values["token"].Present || s.state.Values["token"].Value.Value() != "" {
		t.Fatal("empty string treated as keep/delete")
	}
	if strings.Contains(w.Body.String(), `"token":{"present":true,"value"`) {
		t.Fatal("secret wire value emitted")
	}
	tag := w.Header().Get("ETag")
	w = request(h, "POST", "/admin/settings/prefs/update", `{"revision":"r1","set":{"flag":true},"unset":[]}`, tag)
	check(t, w, 200, "")
	if s.state.Values["token"].Value.Value() != "" {
		t.Fatal("omission did not keep secret")
	}
	// A schema minLength rejects empty replacement without echoing its value.
	s = newStore()
	d := declaration(s)
	d.Groups[0].Fields[1].MinLength = ptr(1)
	cfg := configuration(t, d)
	h, err := adminhttp.NewHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	check(t, request(h, "POST", "/admin/settings/prefs/update", `{"revision":"r1","set":{"token":""},"unset":[]}`, `"v1"`), 422, admin.ValidationFailed)
	if s.state.Values["token"].Value.Value() != "synthetic-private-initial" || s.commits != 0 {
		t.Fatal("invalid secret persisted")
	}
}
func TestAtomicRollbackAcrossGroupAndBadProjection(t *testing.T) {
	for _, mode := range []string{"stage error", "commit error", "empty targets", "different desired"} {
		t.Run(mode, func(t *testing.T) {
			s := newStore()
			s.stageFailure = mode == "stage error"
			s.commitFailure = mode == "commit error"
			s.invalidTargets = mode == "empty targets"
			s.differentStagedValue = mode == "different desired"
			before := copyState(s.state)
			w := request(newHandler(t, s), "POST", "/admin/settings/prefs/update", `{"revision":"r1","set":{"url":"https://new.test","token":"synthetic-private-new"},"unset":[]}`, `"v1"`)
			check(t, w, 503, admin.BackendUnavailable)
			if s.commits != 0 || s.rollbacks != 1 || !reflect.DeepEqual(s.state, before) || s.restarts != 0 || w.Header().Get("ETag") != "" {
				t.Fatal("failure partially persisted/applied or returned staged ETag")
			}
		})
	}
}
func TestNoAutomaticRestartAndPendingNoop(t *testing.T) {
	s := newStore()
	h := newHandler(t, s)
	w := request(h, "POST", "/admin/settings/prefs/update", change, `"v1"`)
	check(t, w, 200, "")
	var response admin.UpdateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.RestartRequired || !reflect.DeepEqual(response.ApplyTargets, []string{"demo"}) || response.Snapshot.Values["url"].ApplyState != admin.PendingRestart || s.restarts != 0 {
		t.Fatal("save applied/restarted or lost pending state")
	}
	tag := w.Header().Get("ETag")
	w = request(h, "POST", "/admin/settings/prefs/update", empty, tag)
	check(t, w, 200, "")
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.RestartRequired || len(response.ApplyTargets) != 0 || len(response.ChangedKeys) != 0 || response.Snapshot.Values["url"].ApplyState != admin.PendingRestart || w.Header().Get("ETag") != tag || s.restarts != 0 {
		t.Fatal("noop carried old restart work or changed ETag")
	}
}
func TestConcurrentWritesOneWinnerNoAutomaticRetry(t *testing.T) {
	s := newStore()
	h := newHandler(t, s)
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, value := range []string{"https://one.test", "https://two.test"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			body := fmt.Sprintf(`{"revision":"r1","set":{"url":%q},"unset":[]}`, value)
			statuses <- request(h, "POST", "/admin/settings/prefs/update", body, `"v1"`).Code
		}(value)
	}
	wg.Wait()
	close(statuses)
	count := map[int]int{}
	for status := range statuses {
		count[status]++
	}
	if count[200] != 1 || count[412] != 1 || s.commits != 1 || s.transactions != 2 || s.stages != 1 {
		t.Fatal("lost update/retry", count, s.commits, s.transactions)
	}
}

func TestExplicitHostPolicyAndCallerFilteredDiscovery(t *testing.T) {
	s := newStore()
	config := configuration(t, declaration(s))
	for _, missing := range []string{"authorize", "resolve", "protect"} {
		bad := config
		switch missing {
		case "authorize":
			bad.Authorize = nil
		case "resolve":
			bad.Resolve = nil
		case "protect":
			bad.ProtectCommand = nil
		}
		if _, err := adminhttp.NewHandler(bad); err == nil {
			t.Fatal("missing host policy accepted", missing)
		}
	}
	resolves, protects := 0, 0
	config.Resolve = func(*http.Request) (*admin.Registry, error) { resolves++; return nil, nil }
	config.ProtectCommand = func(*http.Request) error { protects++; return nil }
	config.Authorize = func(*http.Request, adminhttp.Resource) error {
		return &admin.Failure{Code: admin.Unauthenticated, Message: "Authentication required."}
	}
	h, err := adminhttp.NewHandler(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/admin/settings/prefs/update", "/admin/settings/hidden/update"} {
		check(t, request(h, "POST", path, `not JSON`, ""), 401, admin.Unauthenticated)
	}
	if resolves != 0 || protects != 0 || s.transactions != 0 {
		t.Fatal("unauthorized request reached discovery/backend")
	}
	config = configuration(t, declaration(s))
	config.ProtectCommand = func(*http.Request) error {
		return &admin.Failure{Code: admin.Forbidden, Message: "Command protection rejected."}
	}
	h, _ = adminhttp.NewHandler(config)
	check(t, request(h, "POST", "/admin/settings/prefs/update", change, `"v1"`), 403, admin.Forbidden)
	if s.transactions != 0 {
		t.Fatal("CSRF rejection entered transaction")
	}
	// Host selects a caller-specific filtered declaration before discovery.
	filtered, err := admin.New(admin.Definition{App: admin.App{ID: "demo", Label: "Demo"}, Revision: "caller-view-1"})
	if err != nil {
		t.Fatal(err)
	}
	config = configuration(t, declaration(s))
	config.Resolve = func(*http.Request) (*admin.Registry, error) { return filtered, nil }
	h, _ = adminhttp.NewHandler(config)
	w := request(h, "GET", "/admin/manifest", "", "")
	check(t, w, 200, "")
	if strings.Contains(w.Body.String(), "prefs") {
		t.Fatal("hidden group disclosed")
	}
	check(t, request(h, "GET", "/admin/settings/prefs", "", ""), 404, admin.UnknownResource)
}
func TestStrictCommandsFailBeforeWrites(t *testing.T) {
	badBodies := []string{
		`{"revision":"r1","set":{},"unset":null}`, `{"revision":"r1","set":null,"unset":[]}`,
		`{"revision":"r1","set":{},"unset":[null]}`, `{"revision":"r1","set":{},"unset":[] } {}`,
		`{"revision":"r1","revision":"r1","set":{},"unset":[]}`,
		`{"revision":"r1","set":{"token":"a","token":"b"},"unset":[]}`,
		`{"revision":"r1","set":{},"unset":[],"extra":false}`,
		`{"revision":"r1","set":{"flag":null},"unset":[]}`,
		`{"revision":"r1","set":{"flag":"false"},"unset":[]}`,
		`{"revision":"r1","set":{"flag":[]},"unset":[]}`,
		`{"revision":"r1","set":{"unknown":true},"unset":[]}`,
		`{"revision":"r1","set":{"token":"a"},"unset":["token"]}`,
		`{"revision":"r1","set":{},"unset":["url","url"]}`,
		`{"revision":"r1","set":{"token":"a","\u0074oken":"b"},"unset":[]}`,
	}
	for i, body := range badBodies {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s := newStore()
			check(t, request(newHandler(t, s), "POST", "/admin/settings/prefs/update", body, `"v1"`), 400, admin.MalformedInput)
			if s.stages != 0 || s.commits != 0 {
				t.Fatal("bad body persisted")
			}
		})
	}
	s := newStore()
	config := configuration(t, declaration(s))
	config.MaxBodyBytes = 10
	h, _ := adminhttp.NewHandler(config)
	check(t, request(h, "POST", "/admin/settings/prefs/update", change, `"v1"`), 400, admin.MalformedInput)
	h = newHandler(t, s)
	for _, contentType := range []string{"", "text/plain", "application/json; charset=latin1"} {
		r := httptest.NewRequest("POST", "/admin/settings/prefs/update", strings.NewReader(change))
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		check(t, w, 400, admin.MalformedInput)
	}
	r := httptest.NewRequest("POST", "/admin/settings/prefs/update", strings.NewReader(change))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Add("If-Match", `"v1"`)
	r.Header.Add("If-Match", `"v1"`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	check(t, w, 400, admin.MalformedInput)
}
func TestPermissionsUnsupportedAndMountSafety(t *testing.T) {
	s := newStore()
	h := newHandler(t, s)
	check(t, request(h, "POST", "/admin/settings/prefs/update", `{"revision":"r1","set":{"locked":"changed"},"unset":[]}`, `"v1"`), 403, admin.Forbidden)
	v := s.state.Values["token"]
	v.Editable = false
	v.ReadOnlyReason = "Policy lock"
	s.state.Values["token"] = v
	check(t, request(h, "POST", "/admin/settings/prefs/update", `{"revision":"r1","set":{"token":"synthetic-private-new"},"unset":[]}`, `"v1"`), 403, admin.Forbidden)
	d := declaration(s)
	d.Groups[0].Capabilities.CanReset = false
	config := configuration(t, d)
	h, _ = adminhttp.NewHandler(config)
	check(t, request(h, "POST", "/admin/settings/prefs/reset", `{"revision":"r1","keys":["url"]}`, `"v1"`), 501, admin.Unsupported)
	for _, path := range []string{"/admin/../manifest", "/admin/%6danifest", "/admin/manifest?x=1"} {
		check(t, request(h, "GET", path, "", ""), 400, admin.MalformedInput)
	}
	d = declaration(s)
	d.BasePath = "/sysop"
	config = configuration(t, d)
	h, _ = adminhttp.NewHandler(config)
	check(t, request(h, "GET", "/sysop/admin/manifest", "", ""), 200, "")
	check(t, request(h, "GET", "/admin/manifest", "", ""), 404, admin.UnknownResource)
	config.BasePath = ""
	h, _ = adminhttp.NewHandler(config)
	check(t, request(h, "GET", "/admin/manifest", "", ""), 503, admin.BackendUnavailable)
	config.BasePath = "//unsafe"
	if _, err := adminhttp.NewHandler(config); err == nil {
		t.Fatal("unsafe configured prefix accepted")
	}
	if s.commits != 0 || s.restarts != 0 {
		t.Fatal("denied operations changed settings")
	}
}
func TestObservationCompletedUnhealthyMissingAndUnavailable(t *testing.T) {
	s := newStore()
	d := declaration(s)
	probes := 0
	d.Health = []admin.HealthResource{{ID: "deps", Label: "Dependencies", PollIntervalMS: 1000, StaleAfterMS: 2000, Read: func(context.Context) (admin.HealthObservation, error) {
		probes++
		return admin.HealthObservation{ObservedAt: "2026-10-02T00:00:00Z", Status: admin.Unhealthy, Checks: []admin.HealthCheck{}}, nil
	}}}
	d.Stats = []admin.StatResource{{ID: "active", Label: "Active", PollIntervalMS: 1000, StaleAfterMS: 2000, Unit: admin.Count, Kind: admin.Gauge, Read: func(context.Context) (admin.StatObservation, error) {
		probes++
		return admin.StatObservation{ObservedAt: "2026-10-02T00:00:00Z"}, nil
	}}}
	h, _ := adminhttp.NewHandler(configuration(t, d))
	check(t, request(h, "GET", "/admin/manifest", "", ""), 200, "")
	if probes != 0 {
		t.Fatal("discovery probed")
	}
	w := request(h, "GET", "/admin/health/deps", "", "")
	check(t, w, 200, "")
	if !strings.Contains(w.Body.String(), `"status":"unhealthy"`) {
		t.Fatal("unhealthy lost")
	}
	w = request(h, "GET", "/admin/stats/active", "", "")
	check(t, w, 200, "")
	if !strings.Contains(w.Body.String(), `"value":null`) {
		t.Fatal("missing sample synthesized zero")
	}
	d.Health[0].Read = func(context.Context) (admin.HealthObservation, error) {
		return admin.HealthObservation{}, errors.New("synthetic-private-host-error")
	}
	h, _ = adminhttp.NewHandler(configuration(t, d))
	check(t, request(h, "GET", "/admin/health/deps", "", ""), 503, admin.BackendUnavailable)
	d.Stats[0].Unit = admin.Ratio
	d.Stats[0].Read = func(context.Context) (admin.StatObservation, error) {
		return admin.StatObservation{ObservedAt: "2026-10-02T00:00:00Z", Value: ptr(2.0)}, nil
	}
	h, _ = adminhttp.NewHandler(configuration(t, d))
	check(t, request(h, "GET", "/admin/stats/active", "", ""), 503, admin.BackendUnavailable)
}
func TestValidateRejectsInconsistentPreviewBase(t *testing.T) {
	s := newStore()
	s.previewChanged = true
	check(t, request(newHandler(t, s), "POST", "/admin/settings/prefs/validate", empty, ""), 503, admin.BackendUnavailable)
	if s.commits != 0 || s.stages != 0 {
		t.Fatal("inconsistent preview wrote")
	}
}

func TestUnicodeReplacementsAreNotSilentlyChanged(t *testing.T) {
	for _, replacement := range []string{`\ud800`, `\udc00`, `\ud800\u0041`} {
		s := newStore()
		body := `{"revision":"r1","set":{"token":"` + replacement + `"},"unset":[]}`
		check(t, request(newHandler(t, s), "POST", "/admin/settings/prefs/update", body, `"v1"`), 400, admin.MalformedInput)
		if s.stages != 0 || s.commits != 0 {
			t.Fatal("invalid Unicode persisted")
		}
	}
	for _, replacement := range []string{`\ud83d\udd12`, `\\ud800`, `\ufffd`} {
		s := newStore()
		body := `{"revision":"r1","set":{"token":"` + replacement + `"},"unset":[]}`
		check(t, request(newHandler(t, s), "POST", "/admin/settings/prefs/update", body, `"v1"`), 200, "")
	}
}
func TestRepeatedCallbackAndCancelledStageRollback(t *testing.T) {
	s := newStore()
	s.repeatCallback = true
	before := copyState(s.state)
	check(t, request(newHandler(t, s), "POST", "/admin/settings/prefs/update", change, `"v1"`), 503, admin.BackendUnavailable)
	if s.commits != 0 || s.stages != 1 || !reflect.DeepEqual(before, s.state) {
		t.Fatal("host callback replay committed/retried")
	}
	s = newStore()
	before = copyState(s.state)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.cancelStage = cancel
	r := httptest.NewRequest("POST", "/admin/settings/prefs/update", strings.NewReader(change)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-Match", `"v1"`)
	w := httptest.NewRecorder()
	newHandler(t, s).ServeHTTP(w, r)
	check(t, w, 503, admin.BackendUnavailable)
	if s.commits != 0 || s.rollbacks != 1 || !reflect.DeepEqual(before, s.state) {
		t.Fatal("cancelled request committed")
	}
}

func TestBackendSemanticValidationRequiredAndErrorSanitized(t *testing.T) {
	s := newStore()
	d := declaration(s)
	d.Groups[0].SemanticValidation = func(_ context.Context, values map[string]admin.ResolvedValue) (admin.Validation, error) {
		if values["flag"].Value.Value() == true && values["token"].Value.Value() == "" {
			return admin.Validation{Errors: []admin.ValidationError{{Path: "/token", Code: "combination", Message: "This feature needs a nonempty token."}}}, nil
		}
		return admin.Validation{Valid: true}, nil
	}
	h, _ := adminhttp.NewHandler(configuration(t, d))
	body := `{"revision":"r1","set":{"flag":true,"token":""},"unset":[]}`
	w := request(h, "POST", "/admin/settings/prefs/validate", body, "")
	check(t, w, 200, "")
	var validation admin.Validation
	_ = json.Unmarshal(w.Body.Bytes(), &validation)
	if validation.Valid {
		t.Fatal("semantic failure lost")
	}
	check(t, request(h, "POST", "/admin/settings/prefs/update", body, `"v1"`), 422, admin.ValidationFailed)
	if s.stages != 0 || s.commits != 0 {
		t.Fatal("semantic validation bypassed")
	}
	d.Groups[0].SemanticValidation = func(context.Context, map[string]admin.ResolvedValue) (admin.Validation, error) {
		return admin.Validation{}, errors.New("synthetic-private-semantic-error")
	}
	h, _ = adminhttp.NewHandler(configuration(t, d))
	check(t, request(h, "GET", "/admin/settings/prefs", "", ""), 503, admin.BackendUnavailable)
}

func TestManifestRevisionPrecedesUnknownGroupKeys(t *testing.T) {
	for _, operation := range []string{"validate", "update", "reset"} {
		for _, tt := range []struct {
			name, revision, key string
			status              int
			code                string
		}{
			{"stale unknown", "old", "unknown", 409, admin.ManifestChanged},
			{"current unknown", "r1", "unknown", 400, admin.MalformedInput},
			{"stale valid", "old", "url", 409, admin.ManifestChanged},
		} {
			t.Run(operation+"/"+tt.name, func(t *testing.T) {
				s := newStore()
				before := copyState(s.state)
				body := fmt.Sprintf(`{"revision":%q,"set":{%q:"https://next.test"},"unset":[]}`, tt.revision, tt.key)
				if operation == "reset" {
					body = fmt.Sprintf(`{"revision":%q,"keys":[%q]}`, tt.revision, tt.key)
				}
				check(t, request(newHandler(t, s), "POST", "/admin/settings/prefs/"+operation, body, `"v1"`), tt.status, tt.code)
				if s.previews != 0 || s.stages != 0 || s.commits != 0 || !reflect.DeepEqual(before, s.state) {
					t.Fatal("rejected command resolved or persisted")
				}
			})
		}
	}
}

// Simulate another writer committing after Read and before Preview resolves.
// The helper's validate command itself must never persist its proposed change.
type interleavedWriter struct {
	*store
	written        bool
	changeRevision bool
}

func (s *interleavedWriter) Preview(ctx context.Context, changes admin.Changes) (admin.State, error) {
	s.mu.Lock()
	if !s.written {
		s.written = true
		s.state.Version = "external-v2"
		if s.changeRevision {
			s.state.Revision = "r2"
		}
		v := s.state.Values["token"]
		v.Value = scalar("synthetic-private SELECT token FROM secrets /private/credentials")
		s.state.Values["token"] = v
	}
	s.mu.Unlock()
	return s.store.Preview(ctx, changes)
}

func TestValidateInterleavedWriteUnavailableAndRetry(t *testing.T) {
	s := &interleavedWriter{store: newStore()}
	d := declaration(s.store)
	d.Groups[0].Backend = s
	h, err := adminhttp.NewHandler(configuration(t, d))
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/admin/settings/prefs/validate", change, "")
	check(t, w, 503, admin.BackendUnavailable)
	want := `{"error":{"code":"backend_unavailable","message":"The admin backend is unavailable."}}`
	if w.Body.String() != want || w.Header().Get("ETag") != "" {
		t.Fatal("unavailable response disclosed values/details or a completed snapshot", w.Body.String())
	}
	if !s.written || s.state.Version != "external-v2" || s.state.Values["url"].Value.Value() != "https://initial.test" || s.reads != 1 || s.previews != 1 || s.transactions != 0 || s.stages != 0 || s.commits != 0 || s.restarts != 0 {
		t.Fatal("interleaved validation persisted, applied, or retried")
	}
	// An explicit retry observes the new stable base and completes without writes.
	before := copyState(s.state)
	w = request(h, "POST", "/admin/settings/prefs/validate", change, "")
	check(t, w, 200, "")
	var validation admin.Validation
	if err := json.Unmarshal(w.Body.Bytes(), &validation); err != nil || !validation.Valid {
		t.Fatal("stable retry did not complete validation", w.Body.String(), err)
	}
	if s.reads != 2 || s.previews != 2 || s.transactions != 0 || s.stages != 0 || s.commits != 0 || s.restarts != 0 || !reflect.DeepEqual(before, s.state) {
		t.Fatal("retry changed persistent state")
	}
}

func TestValidateInterleavedDeclarationRevisionChanged(t *testing.T) {
	s := &interleavedWriter{store: newStore(), changeRevision: true}
	d := declaration(s.store)
	d.Groups[0].Backend = s
	h, err := adminhttp.NewHandler(configuration(t, d))
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/admin/settings/prefs/validate", change, "")
	check(t, w, 409, admin.ManifestChanged)
	if s.reads != 1 || s.previews != 1 || s.state.Revision != "r2" || s.state.Values["url"].Value.Value() != "https://initial.test" || s.transactions != 0 || s.stages != 0 || s.commits != 0 || s.restarts != 0 || w.Header().Get("ETag") != "" {
		t.Fatal("declaration race completed or persisted validation")
	}
}
