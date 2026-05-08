// Package envelopes is the shared Go primitive library for the Envelope UI
// Protocol. It defines the canonical Envelope and Response wire types,
// loads the core type catalog from the embedded YAML manifest, validates
// payloads against per-type JSON Schemas, and exposes a plugin extension
// API for runtime registration of additional envelope types.
//
// The package is transport-agnostic. It does not bind to MCP, SSE, or any
// other transport — those concerns live in consumer apps. It also does not
// persist envelope instances; storage is per-host.
//
// The companion ts-envelopes package consumes the same manifest from the
// repo root for parity between Go and TypeScript hosts.
package envelopes
