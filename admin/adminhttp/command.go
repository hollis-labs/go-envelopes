package adminhttp

import (
	"encoding/json"
	"net/http"

	"github.com/hollis-labs/go-envelopes/admin"
)

func (h *handler) command(w http.ResponseWriter, r *http.Request, g admin.Group, revision, operation string) {
	supported := operation == "validate" && g.Capabilities.CanValidate || operation == "update" && g.Capabilities.CanUpdate || operation == "reset" && g.Capabilities.CanReset
	if !supported {
		writeFailure(w, failure(admin.Unsupported))
		return
	}
	command, err := decodeCommand(r, operation, h.config.MaxBodyBytes)
	if err != nil {
		writeFailure(w, err)
		return
	}
	if operation == "validate" {
		h.validate(w, r, g, revision, command)
		return
	}
	var body []byte
	var tag string
	invoked := false
	err = g.Backend.WithTransaction(r.Context(), func(tx admin.GroupTransaction) error {
		if invoked || tx == nil {
			return failure(admin.BackendUnavailable)
		}
		invoked = true
		if r.Context().Err() != nil {
			return failure(admin.BackendUnavailable)
		}
		current, err := tx.Current()
		if err != nil {
			return err
		}
		current = detached(current)
		if f := g.CheckChanges(current, command.Changes); f != nil {
			return f
		}
		if command.Revision != revision || current.Revision != revision {
			return failure(admin.ManifestChanged)
		}
		tag, err = current.ETag()
		if err != nil {
			return err
		}
		if f := precondition(r, tag); f != nil {
			return f
		}
		candidate, err := tx.Resolve(command.Changes)
		if err != nil {
			return err
		}
		if candidate.Revision != revision {
			return failure(admin.ManifestChanged)
		}
		if candidate.Version != current.Version {
			return failure(admin.BackendUnavailable)
		}
		candidate = detached(candidate)
		validation, err := g.ValidateCandidate(r.Context(), candidate)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return &admin.Failure{Code: admin.ValidationFailed, Message: "The desired configuration is invalid.", Errors: validation.Errors}
		}
		staged, err := tx.Stage(command.Changes)
		if err != nil {
			return err
		}
		if staged.State.Revision != revision {
			return failure(admin.ManifestChanged)
		}
		if !sameDesired(candidate, staged.State) {
			return failure(admin.BackendUnavailable)
		}
		response, err := g.UpdateResult(r.Context(), current, staged)
		if err != nil {
			return err
		}
		tag, err = staged.State.ETag()
		if err != nil {
			return err
		}
		body, err = json.Marshal(response)
		if err != nil {
			return err
		}
		if r.Context().Err() != nil {
			return failure(admin.BackendUnavailable)
		}
		return nil // Only now may the host atomically commit the staged state.
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	if body == nil {
		writeFailure(w, failure(admin.BackendUnavailable))
		return
	}
	w.Header().Set("ETag", tag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
func (h *handler) validate(w http.ResponseWriter, r *http.Request, g admin.Group, revision string, command admin.Command) {
	current, err := g.Backend.Read(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	if f := g.CheckChanges(current, command.Changes); f != nil {
		writeFailure(w, f)
		return
	}
	if command.Revision != revision || current.Revision != revision {
		writeFailure(w, failure(admin.ManifestChanged))
		return
	}
	candidate, err := g.Backend.Preview(r.Context(), command.Changes)
	if err != nil {
		writeFailure(w, err)
		return
	}
	if candidate.Revision != revision {
		writeFailure(w, failure(admin.ManifestChanged))
		return
	}
	// Preview carries the BASE opaque version, not a speculative committed token.
	// A changed base cannot yield a completed validation of this read's permissions.
	if candidate.Version != current.Version {
		writeFailure(w, failure(admin.BackendUnavailable))
		return
	}
	validation, err := g.ValidateCandidate(r.Context(), candidate)
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, validation)
}
func sameDesired(a, b admin.State) bool {
	if len(a.Values) != len(b.Values) {
		return false
	}
	for key, v := range a.Values {
		other, ok := b.Values[key]
		if !ok || v.Present != other.Present || v.Value.Value() != other.Value.Value() {
			return false
		}
	}
	return true
}

func detached(s admin.State) admin.State {
	values := make(map[string]admin.ResolvedValue, len(s.Values))
	for key, v := range s.Values {
		if v.Source != nil {
			source := *v.Source
			v.Source = &source
		}
		values[key] = v
	}
	s.Values = values
	return s
}
