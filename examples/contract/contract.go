// Package contract demonstrates wiring envelopestest.RunContract into
// a host's test suite. The helper exercises every v1 invariant against
// a registry; downstream consumers should run it from their own
// *_test.go to gate go-envelopes upgrades.
//
// Run the contract:
//
//	go test ./examples/contract
//
// In a real host, the same wiring lives next to the host's other
// integration tests (e.g. inside the package that constructs the
// host's Registry).
package contract
