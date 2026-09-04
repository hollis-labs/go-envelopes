package envelopes

import (
	"encoding/json"
	"testing"
)

func TestCancellationVocabularyV04CompatibilityWindow(t *testing.T) {
	if got, want := ResponseStatusCanceled, ResponseStatus("canceled"); got != want {
		t.Fatalf("ResponseStatusCanceled = %q, want %q", got, want)
	}
	if got, want := ErrorCodeUserCanceled, "user-canceled"; got != want {
		t.Fatalf("ErrorCodeUserCanceled = %q, want %q", got, want)
	}

	// v0.2 emitted these British-spelled values. Keep their exported symbols
	// and exact wire values intact for the explicitly bounded v0.4.x window.
	if got, want := ResponseStatusCancelled, ResponseStatus("cancelled"); got != want {
		t.Fatalf("ResponseStatusCancelled = %q, want legacy value %q", got, want)
	}
	if got, want := ErrorCodeUserCancelled, "user-cancelled"; got != want {
		t.Fatalf("ErrorCodeUserCancelled = %q, want legacy value %q", got, want)
	}

	if !ResponseStatusCanceled.IsCanonical() || !ResponseStatusCanceled.IsValid() {
		t.Fatal("canonical canceled status is not canonical and valid")
	}
	if ResponseStatusCancelled.IsCanonical() || !ResponseStatusCancelled.IsValid() {
		t.Fatal("legacy cancelled status must be recognized but non-canonical in v0.4.x")
	}
	if got := ResponseStatusCancelled.Canonical(); got != ResponseStatusCanceled {
		t.Fatalf("legacy status canonicalized to %q, want %q", got, ResponseStatusCanceled)
	}
	if got := CanonicalErrorCode(ErrorCodeUserCancelled); got != ErrorCodeUserCanceled {
		t.Fatalf("legacy error code canonicalized to %q, want %q", got, ErrorCodeUserCanceled)
	}
	if got := CanonicalErrorCode("vendor-extension"); got != "vendor-extension" {
		t.Fatalf("extension error code changed to %q", got)
	}
	if ResponseStatus("unknown").IsValid() {
		t.Fatal("unknown response status is valid")
	}
}

func TestCancellationVocabularyReadsV02ResponseJSON(t *testing.T) {
	raw := []byte(`{
		"v": 1,
		"envelopeId": "legacy-response",
		"kind": "error",
		"status": "cancelled",
		"error": {"code": "user-cancelled", "message": "stopped by user"}
	}`)
	var response Response
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode v0.2 response: %v", err)
	}
	if response.Status != ResponseStatusCancelled || response.Error == nil || response.Error.Code != ErrorCodeUserCancelled {
		t.Fatalf("decoded v0.2 response = %#v", response)
	}

	registry := mustLoad(t)
	if err := registry.ValidateResponse("info-card", &response); err != nil {
		t.Fatalf("v0.2 response rejected during v0.4.x compatibility window: %v", err)
	}
	response.Status = response.Status.Canonical()
	response.Error.Code = CanonicalErrorCode(response.Error.Code)
	if response.Status != ResponseStatusCanceled || response.Error.Code != ErrorCodeUserCanceled {
		t.Fatalf("canonicalized response = %#v", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["status"] != "canceled" {
		t.Fatalf("canonical response status on wire = %#v", wire["status"])
	}
	errorObject, _ := wire["error"].(map[string]any)
	if errorObject["code"] != "user-canceled" {
		t.Fatalf("canonical response error code on wire = %#v", errorObject["code"])
	}
}
