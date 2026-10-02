package admin

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
)

type HealthStatus string

const (
	Healthy       HealthStatus = "healthy"
	Degraded      HealthStatus = "degraded"
	Unhealthy     HealthStatus = "unhealthy"
	UnknownHealth HealthStatus = "unknown"
)

type Unit string

const (
	Count        Unit = "count"
	Bytes        Unit = "bytes"
	Seconds      Unit = "seconds"
	Milliseconds Unit = "milliseconds"
	Ratio        Unit = "ratio"
	Percent      Unit = "percent"
)

type StatKind string

const (
	Gauge   StatKind = "gauge"
	Counter StatKind = "counter"
)

type ObservationDeclaration struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	Section        string   `json:"section"`
	Endpoint       Endpoint `json:"endpoint"`
	PollIntervalMS int      `json:"poll_interval_ms"`
	StaleAfterMS   int      `json:"stale_after_ms"`
}
type StatDeclaration struct {
	ObservationDeclaration
	Unit Unit     `json:"unit"`
	Kind StatKind `json:"kind"`
}
type HealthCheck struct {
	ID      string       `json:"id"`
	Label   string       `json:"label"`
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
}
type HealthObservation struct {
	ObservedAt string        `json:"observed_at"`
	Status     HealthStatus  `json:"status"`
	Checks     []HealthCheck `json:"checks"`
}
type StatObservation struct {
	ObservedAt string   `json:"observed_at"`
	Value      *float64 `json:"value"`
}

// Read callbacks must be authorized by the host before invocation and must be
// cheap/cached, read-only and return sanitized messages. Discovery never probes.
type HealthResource struct {
	ID             string
	Label          string
	PollIntervalMS int
	StaleAfterMS   int
	Read           func(context.Context) (HealthObservation, error)
}
type StatResource struct {
	ID             string
	Label          string
	PollIntervalMS int
	StaleAfterMS   int
	Unit           Unit
	Kind           StatKind
	Read           func(context.Context) (StatObservation, error)
}

func observationDeclaration(id, label, path string, poll, stale int) ObservationDeclaration {
	return ObservationDeclaration{ID: id, Label: label, Section: "status", Endpoint: Endpoint{Method: "GET", Path: path}, PollIntervalMS: poll, StaleAfterMS: stale}
}
func validObservationDeclaration(d ObservationDeclaration) bool {
	return validID(d.ID) && nonempty(d.Label) && SafePath(d.Endpoint.Path) && d.PollIntervalMS >= 1000 && d.StaleAfterMS >= d.PollIntervalMS && uint64(d.StaleAfterMS) <= MaxSafeInteger
}
func validUnit(u Unit) bool {
	return u == Count || u == Bytes || u == Seconds || u == Milliseconds || u == Ratio || u == Percent
}
func validKind(k StatKind) bool { return k == Gauge || k == Counter }
func validHealth(s HealthStatus) bool {
	return s == Healthy || s == Degraded || s == Unhealthy || s == UnknownHealth
}
func validTime(v string) bool {
	_, err := time.Parse(time.RFC3339Nano, v)
	return err == nil && strings.HasSuffix(v, "Z")
}

// Validate treats unhealthy as a completed observation, not backend unavailable.
func (o HealthObservation) Validate() error {
	if !validTime(o.ObservedAt) || !validHealth(o.Status) || o.Checks == nil {
		return errors.New("admin: invalid health observation")
	}
	seen := map[string]bool{}
	for _, c := range o.Checks {
		if !validID(c.ID) || !nonempty(c.Label) || !validHealth(c.Status) || seen[c.ID] {
			return errors.New("admin: invalid health check")
		}
		seen[c.ID] = true
	}
	return nil
}
func (o StatObservation) Validate(unit Unit) error {
	if !validTime(o.ObservedAt) || !validUnit(unit) {
		return errors.New("admin: invalid stat observation")
	}
	if o.Value == nil {
		return nil
	} // Explicit null is a missing sample, not zero.
	v := *o.Value
	if math.IsNaN(v) || math.IsInf(v, 0) || (unit == Ratio && (v < 0 || v > 1)) || (unit == Percent && (v < 0 || v > 100)) {
		return errors.New("admin: invalid stat sample")
	}
	return nil
}
