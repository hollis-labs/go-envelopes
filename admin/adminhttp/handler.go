// Package adminhttp binds admin contract v1 to HTTP. It starts no server,
// goroutine or timer and owns no storage, authorization policy or lifecycle.
package adminhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/hollis-labs/go-envelopes/admin"
)

// Resource names the requested operation before any resource lookup. The host
// obtains the authenticated actor and selected app context from the request.
type Resource struct {
	Kind      string
	ID        string
	Operation string
}

// Config requires explicit host policy; there are no allow-all defaults.
type Config struct {
	// BasePath must match the deployment prefix used in every resolved registry.
	BasePath string
	// Authorize authenticates and authorizes EVERY request, including discovery
	// and unknown resources, before lookup. Only sanitized Failure messages are
	// returned; arbitrary errors are mapped to a fixed unavailable response.
	Authorize func(*http.Request, Resource) error
	// Resolve returns a caller-filtered immutable registry for the selected app
	// context. It is metadata-only: no probing or mutation during discovery.
	Resolve func(*http.Request) (*admin.Registry, error)
	// ProtectCommand enforces the app's CSRF policy on every POST. Token-only
	// hosts may explicitly supply a no-op once their host policy makes it safe.
	ProtectCommand func(*http.Request) error
	// MaxBodyBytes bounds command reads. Zero uses 64 KiB.
	MaxBodyBytes int64
}
type handler struct {
	config Config
	prefix string
}

func NewHandler(config Config) (http.Handler, error) {
	if config.Authorize == nil || config.Resolve == nil || config.ProtectCommand == nil || config.MaxBodyBytes < 0 || (config.BasePath != "" && !admin.SafePath(config.BasePath)) {
		return nil, errors.New("adminhttp: explicit host policies and canonical prefix required")
	}
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = 64 * 1024
	}
	if config.MaxBodyBytes == int64(^uint64(0)>>1) {
		return nil, errors.New("adminhttp: invalid body limit")
	}
	return &handler{config: config, prefix: config.BasePath + "/admin"}, nil
}
func failure(code string) *admin.Failure {
	messages := map[string]string{
		admin.MalformedInput: "Malformed admin request.", admin.Unauthenticated: "Authentication is required.", admin.Forbidden: "The requested operation is forbidden.", admin.UnknownResource: "Unknown admin resource.", admin.ManifestChanged: "The admin manifest changed; refetch and reconcile.", admin.ValueConflict: "The settings changed; refetch and reconcile.", admin.ValidationFailed: "The desired configuration is invalid.", admin.PreconditionRequired: "If-Match with the read ETag is required.", admin.Unsupported: "The requested operation is unsupported.", admin.BackendUnavailable: "The admin backend is unavailable.",
	}
	return &admin.Failure{Code: code, Message: messages[code]}
}
func safeFailure(err error) *admin.Failure {
	var f *admin.Failure
	if errors.As(err, &f) && f != nil {
		switch f.Code {
		case admin.MalformedInput, admin.Unauthenticated, admin.Forbidden, admin.UnknownResource, admin.ManifestChanged, admin.ValueConflict, admin.ValidationFailed, admin.PreconditionRequired, admin.Unsupported, admin.BackendUnavailable:
			// Hosts explicitly author sanitized messages/details. Detach the slice.
			return &admin.Failure{Code: f.Code, Message: f.Message, Errors: append([]admin.ValidationError(nil), f.Errors...)}
		}
	}
	return failure(admin.BackendUnavailable)
}
func writeFailure(w http.ResponseWriter, err error) {
	f := safeFailure(err)
	b, _ := json.Marshal(admin.ErrorResponse{Error: *f})
	w.WriteHeader(f.StatusCode())
	_, _ = w.Write(b)
}
func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		writeFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
func (h *handler) route(r *http.Request) (Resource, string, *admin.Failure) {
	path := r.URL.Path
	// Decode no escaping and normalize no paths: declarations are canonical.
	if !admin.SafePath(path) || r.URL.EscapedPath() != path || r.URL.RawQuery != "" {
		return Resource{Kind: "unknown"}, "", failure(admin.MalformedInput)
	}
	if !strings.HasPrefix(path, h.prefix+"/") {
		return Resource{Kind: "unknown"}, "", failure(admin.UnknownResource)
	}
	parts := strings.Split(strings.TrimPrefix(path, h.prefix+"/"), "/")
	if len(parts) == 1 && parts[0] == "manifest" {
		return Resource{Kind: "manifest", Operation: "read"}, "GET", nil
	}
	if len(parts) >= 2 && (parts[0] == "settings" || parts[0] == "health" || parts[0] == "stats") {
		resource := Resource{Kind: parts[0], ID: parts[1], Operation: "read"}
		if len(parts) == 2 {
			return resource, "GET", nil
		}
		if resource.Kind == "settings" && len(parts) == 3 && (parts[2] == "validate" || parts[2] == "update" || parts[2] == "reset") {
			resource.Operation = parts[2]
			return resource, "POST", nil
		}
	}
	return Resource{Kind: "unknown"}, "", failure(admin.UnknownResource)
}
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	resource, method, routeFailure := h.route(r)
	if err := h.config.Authorize(r, resource); err != nil {
		writeFailure(w, err)
		return
	}
	if routeFailure != nil {
		writeFailure(w, routeFailure)
		return
	}
	if r.Method != method {
		writeFailure(w, failure(admin.MalformedInput))
		return
	}
	if method == "POST" {
		if err := h.config.ProtectCommand(r); err != nil {
			writeFailure(w, err)
			return
		}
	}
	registry, err := h.config.Resolve(r)
	if err != nil {
		writeFailure(w, err)
		return
	}
	if registry == nil {
		writeFailure(w, failure(admin.BackendUnavailable))
		return
	}
	manifest := registry.Manifest()
	if !h.matchesMount(manifest) {
		writeFailure(w, failure(admin.BackendUnavailable))
		return
	}
	switch resource.Kind {
	case "manifest":
		writeJSON(w, manifest)
	case "settings":
		group, ok := registry.Group(resource.ID)
		if !ok {
			writeFailure(w, failure(admin.UnknownResource))
			return
		}
		if resource.Operation == "read" {
			h.read(w, r, group, manifest.Revision)
			return
		}
		h.command(w, r, group, manifest.Revision, resource.Operation)
	case "health":
		health, ok := registry.Health(resource.ID)
		if !ok {
			writeFailure(w, failure(admin.UnknownResource))
			return
		}
		observation, err := health.Read(r.Context())
		if err == nil {
			err = observation.Validate()
		}
		if err != nil {
			writeFailure(w, err)
			return
		}
		writeJSON(w, observation)
	case "stats":
		stat, ok := registry.Stat(resource.ID)
		if !ok {
			writeFailure(w, failure(admin.UnknownResource))
			return
		}
		observation, err := stat.Read(r.Context())
		if err == nil {
			err = observation.Validate(stat.Unit)
		}
		if err != nil {
			writeFailure(w, err)
			return
		}
		writeJSON(w, observation)
	}
}
func (h *handler) matchesMount(m admin.Manifest) bool {
	for _, g := range m.Settings {
		base := h.prefix + "/settings/" + g.ID
		endpoints := []struct {
			endpoint     *admin.Endpoint
			method, path string
		}{{g.Endpoints.Read, "GET", base}, {g.Endpoints.Validate, "POST", base + "/validate"}, {g.Endpoints.Update, "POST", base + "/update"}, {g.Endpoints.Reset, "POST", base + "/reset"}}
		for _, e := range endpoints {
			if e.endpoint != nil && (e.endpoint.Method != e.method || e.endpoint.Path != e.path) {
				return false
			}
		}
	}
	for _, o := range m.Health {
		if o.Endpoint.Method != "GET" || o.Endpoint.Path != h.prefix+"/health/"+o.ID {
			return false
		}
	}
	for _, o := range m.Stats {
		if o.Endpoint.Method != "GET" || o.Endpoint.Path != h.prefix+"/stats/"+o.ID {
			return false
		}
	}
	return true
}
func (h *handler) read(w http.ResponseWriter, r *http.Request, g admin.Group, revision string) {
	if !g.Capabilities.CanRead {
		writeFailure(w, failure(admin.Unsupported))
		return
	}
	state, err := g.Backend.Read(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	if state.Revision != revision {
		writeFailure(w, failure(admin.ManifestChanged))
		return
	}
	snapshot, err := g.Project(r.Context(), state)
	if err != nil {
		writeFailure(w, err)
		return
	}
	tag, err := state.ETag()
	if err != nil {
		writeFailure(w, err)
		return
	}
	w.Header().Set("ETag", tag)
	writeJSON(w, snapshot)
}
